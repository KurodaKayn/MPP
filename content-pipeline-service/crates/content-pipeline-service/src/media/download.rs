use std::net::{IpAddr, Ipv4Addr, Ipv6Addr, SocketAddr};
use std::time::Duration;

use futures_util::StreamExt;
use reqwest::header::{CONTENT_TYPE, LOCATION};
use reqwest::redirect::Policy;
use reqwest::{Client, Response as HttpResponse, Url};
use tokio::time::timeout;
use tonic::Status;

const MEDIA_DOWNLOAD_TIMEOUT: Duration = Duration::from_secs(20);
const MEDIA_REDIRECT_LIMIT: usize = 3;

#[derive(Debug)]
struct ValidatedMediaUrl {
    url: Url,
    resolved_host: Option<ResolvedHost>,
}

// For hostnames, keep the DNS answer that passed SSRF validation and hand it
// to reqwest. That avoids validating one address and connecting to another.
#[derive(Debug)]
struct ResolvedHost {
    host: String,
    addrs: Vec<SocketAddr>,
}

fn media_http_client(resolved_host: Option<&ResolvedHost>) -> Result<Client, reqwest::Error> {
    let builder = Client::builder()
        .timeout(MEDIA_DOWNLOAD_TIMEOUT)
        .redirect(Policy::none());

    match resolved_host {
        Some(resolved_host) => builder
            .resolve_to_addrs(&resolved_host.host, &resolved_host.addrs)
            .build(),
        None => builder.build(),
    }
}

pub(super) async fn fetch_media_url(mut url: Url) -> Result<HttpResponse, Status> {
    let mut redirects = 0;

    loop {
        // Every redirect target is revalidated so a public URL cannot bounce
        // the downloader into a private or otherwise non-global address.
        let validated = validate_media_url(url).await?;
        let client = media_http_client(validated.resolved_host.as_ref())
            .map_err(|_| Status::internal("failed to build media HTTP client"))?;
        let response = client
            .get(validated.url)
            .send()
            .await
            .map_err(media_download_error_to_status)?;

        if !response.status().is_redirection() {
            return Ok(response);
        }

        if redirects >= MEDIA_REDIRECT_LIMIT {
            return Err(Status::invalid_argument("media redirect limit exceeded"));
        }
        redirects += 1;

        let base_url = response.url().clone();
        let location = response
            .headers()
            .get(LOCATION)
            .and_then(|value| value.to_str().ok())
            .ok_or_else(|| Status::invalid_argument("media redirect missing location"))?;
        url = base_url
            .join(location)
            .map_err(|_| Status::invalid_argument("invalid media redirect URL"))?;
    }
}

pub(super) async fn read_limited_body(
    response: reqwest::Response,
    max_bytes: u64,
) -> Result<Vec<u8>, Status> {
    if let Some(content_length) = response.content_length()
        && content_length > max_bytes
    {
        return Err(Status::resource_exhausted(format!(
            "media exceeds max bytes: {content_length} > {max_bytes}"
        )));
    }

    let mut body = Vec::new();
    let mut stream = response.bytes_stream();
    while let Some(chunk) = stream.next().await {
        let chunk = chunk.map_err(media_download_error_to_status)?;
        let next_len = body
            .len()
            .checked_add(chunk.len())
            .ok_or_else(|| Status::resource_exhausted("media body is too large"))?;
        let next_len = u64::try_from(next_len)
            .map_err(|_| Status::resource_exhausted("media body is too large"))?;
        if next_len > max_bytes {
            return Err(Status::resource_exhausted(format!(
                "media exceeds max bytes: {next_len} > {max_bytes}"
            )));
        }
        body.extend_from_slice(&chunk);
    }

    Ok(body)
}

pub(super) fn response_content_type(response: &reqwest::Response) -> Option<String> {
    response
        .headers()
        .get(CONTENT_TYPE)
        .and_then(|value| value.to_str().ok())
        .and_then(|value| value.split(';').next())
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(ToString::to_string)
}

fn media_download_error_to_status(err: reqwest::Error) -> Status {
    if err.is_timeout() {
        Status::deadline_exceeded("media download timed out")
    } else if err.is_redirect() {
        Status::invalid_argument("unsafe media redirect")
    } else {
        Status::unavailable("failed to download media")
    }
}

async fn validate_media_url(mut url: Url) -> Result<ValidatedMediaUrl, Status> {
    if url.scheme() != "https" {
        return Err(Status::invalid_argument("unsafe media URL"));
    }

    let host = normalized_media_host(&url)?;
    if is_blocked_hostname(&host) {
        return Err(Status::invalid_argument("unsafe media URL"));
    }

    if let Ok(ip) = host.parse::<IpAddr>() {
        if is_public_ip(ip) {
            return Ok(ValidatedMediaUrl {
                url,
                resolved_host: None,
            });
        }
        return Err(Status::invalid_argument("unsafe media URL"));
    }

    url.set_host(Some(&host))
        .map_err(|_| Status::invalid_argument("invalid media URL"))?;
    let port = url
        .port_or_known_default()
        .ok_or_else(|| Status::invalid_argument("invalid media URL port"))?;
    let addrs = resolve_host_addrs(&host, port).await?;
    validate_resolved_addrs(&host, &addrs)?;

    Ok(ValidatedMediaUrl {
        url,
        resolved_host: Some(ResolvedHost { host, addrs }),
    })
}

fn normalized_media_host(url: &Url) -> Result<String, Status> {
    let Some(host) = url.host_str() else {
        return Err(Status::invalid_argument("invalid media URL host"));
    };

    let host = host.trim_end_matches('.');
    Ok(host
        .strip_prefix('[')
        .and_then(|value| value.strip_suffix(']'))
        .unwrap_or(host)
        .to_ascii_lowercase())
}

fn is_blocked_hostname(host: &str) -> bool {
    host == "localhost" || host.ends_with(".localhost")
}

async fn resolve_host_addrs(host: &str, port: u16) -> Result<Vec<SocketAddr>, Status> {
    let lookup = timeout(
        MEDIA_DOWNLOAD_TIMEOUT,
        tokio::net::lookup_host((host, port)),
    )
    .await
    .map_err(|_| Status::deadline_exceeded("media DNS lookup timed out"))?
    .map_err(|_| Status::unavailable("failed to resolve media host"))?;
    let addrs = lookup.collect::<Vec<_>>();
    if addrs.is_empty() {
        return Err(Status::unavailable("media host did not resolve"));
    }

    Ok(addrs)
}

fn validate_resolved_addrs(host: &str, addrs: &[SocketAddr]) -> Result<(), Status> {
    if addrs.iter().all(|addr| is_public_ip(addr.ip())) {
        return Ok(());
    }

    Err(Status::invalid_argument(format!(
        "media host {host} resolved to a private address"
    )))
}

fn is_public_ip(ip: IpAddr) -> bool {
    match ip {
        IpAddr::V4(ip) => is_public_ipv4(ip),
        IpAddr::V6(ip) => is_public_ipv6(ip),
    }
}

fn is_public_ipv4(ip: Ipv4Addr) -> bool {
    let [first, second, third, _] = ip.octets();

    // Treat "public" as globally routable, not just "not RFC1918".
    !(first == 0
        || first == 10
        || first == 127
        || (first == 100 && (64..=127).contains(&second))
        || (first == 169 && second == 254)
        || (first == 172 && (16..=31).contains(&second))
        || (first == 192 && second == 0 && third == 0)
        || (first == 192 && second == 0 && third == 2)
        || (first == 192 && second == 168)
        || (first == 198 && (second == 18 || second == 19))
        || (first == 198 && second == 51 && third == 100)
        || (first == 203 && second == 0 && third == 113)
        || first >= 224)
}

fn is_public_ipv6(ip: Ipv6Addr) -> bool {
    if let Some(ipv4) = ip.to_ipv4_mapped() {
        return is_public_ipv4(ipv4);
    }

    // Keep this list explicit because media downloads are user-directed SSRF
    // entry points; special-use ranges should fail closed.
    let first_segment = ip.segments()[0];
    let second_segment = ip.segments()[1];
    let is_unique_local = (first_segment & 0xfe00) == 0xfc00;
    let is_link_local = (first_segment & 0xffc0) == 0xfe80;
    let is_multicast = (first_segment & 0xff00) == 0xff00;
    let is_documentation = first_segment == 0x2001 && second_segment == 0x0db8;
    let is_benchmark = first_segment == 0x2001 && second_segment == 0x0002;
    let is_discard_prefix =
        first_segment == 0x0100 && ip.segments()[1..4].iter().all(|segment| *segment == 0);

    !(ip.is_loopback()
        || ip.is_unspecified()
        || is_unique_local
        || is_link_local
        || is_multicast
        || is_documentation
        || is_benchmark
        || is_discard_prefix)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn url(value: &str) -> Url {
        Url::parse(value).expect("test URL should parse")
    }

    #[test]
    fn accepts_public_resolved_addresses() {
        let addrs = vec!["93.184.216.34:443".parse().expect("address should parse")];

        validate_resolved_addrs("example.com", &addrs)
            .expect("public resolved address should be accepted");
    }

    #[test]
    fn rejects_private_resolved_addresses() {
        let addrs = vec![
            "93.184.216.34:443".parse().expect("address should parse"),
            "169.254.169.254:443".parse().expect("address should parse"),
        ];

        let err = validate_resolved_addrs("attacker.example", &addrs)
            .expect_err("private resolved address should be rejected");

        assert_eq!(err.code(), tonic::Code::InvalidArgument);
    }

    #[tokio::test]
    async fn rejects_non_https_media_url() {
        let err = validate_media_url(url("http://example.com/image.png"))
            .await
            .expect_err("non-https URL should be rejected");

        assert_eq!(err.code(), tonic::Code::InvalidArgument);
    }

    #[tokio::test]
    async fn rejects_localhost_media_url() {
        let err = validate_media_url(url("https://localhost/image.png"))
            .await
            .expect_err("localhost URL should be rejected");
        assert_eq!(err.code(), tonic::Code::InvalidArgument);

        let err = validate_media_url(url("https://assets.localhost/image.png"))
            .await
            .expect_err("localhost subdomain URL should be rejected");
        assert_eq!(err.code(), tonic::Code::InvalidArgument);
    }

    #[tokio::test]
    async fn rejects_private_ip_media_url() {
        for value in [
            "https://127.0.0.1/image.png",
            "https://10.0.0.1/image.png",
            "https://192.168.1.10/image.png",
            "https://[::1]/image.png",
            "https://[fd00::1]/image.png",
            "https://[::ffff:127.0.0.1]/image.png",
        ] {
            let err = validate_media_url(url(value))
                .await
                .expect_err("private IP URL should be rejected");
            assert_eq!(err.code(), tonic::Code::InvalidArgument);
        }
    }

    #[tokio::test]
    async fn rejects_non_global_ip_media_url() {
        for value in [
            "https://198.18.0.1/image.png",
            "https://224.0.0.1/image.png",
            "https://240.0.0.1/image.png",
            "https://[ff02::1]/image.png",
        ] {
            let err = validate_media_url(url(value))
                .await
                .expect_err("non-global IP URL should be rejected");
            assert_eq!(err.code(), tonic::Code::InvalidArgument);
        }
    }
}
