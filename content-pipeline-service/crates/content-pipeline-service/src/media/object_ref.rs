use std::time::Duration;

use reqwest::Client;
use serde::{Deserialize, Serialize};
use tonic::Status;

const OBJECT_REF_RESOLVER_TIMEOUT: Duration = Duration::from_secs(10);
const OBJECT_REF_RESOLVER_URL_ENV: &str = "CONTENT_PIPELINE_MEDIA_RESOLVER_URL";
const INTERNAL_TOKEN_ENV: &str = "CONTENT_PIPELINE_INTERNAL_TOKEN";

// Object refs are durable internal handles. The resolver converts them into
// short-lived URLs without exposing storage credentials to callers.
#[derive(Debug, Clone)]
pub(super) struct ObjectRefResolverConfig {
    pub(super) url: String,
    pub(super) internal_token: String,
}

impl ObjectRefResolverConfig {
    pub(super) fn from_env() -> Option<Self> {
        let url = std::env::var(OBJECT_REF_RESOLVER_URL_ENV)
            .ok()
            .map(|value| value.trim().to_string())
            .filter(|value| !value.is_empty())?;
        let internal_token = std::env::var(INTERNAL_TOKEN_ENV)
            .ok()
            .map(|value| value.trim().to_string())
            .filter(|value| !value.is_empty())?;
        Some(Self {
            url,
            internal_token,
        })
    }
}

pub(super) fn resolver_http_client() -> Result<Client, reqwest::Error> {
    Client::builder()
        .timeout(OBJECT_REF_RESOLVER_TIMEOUT)
        .build()
}

#[derive(Debug, Serialize)]
struct ResolveObjectRefRequest<'a> {
    object_ref: &'a str,
}

#[derive(Debug, Deserialize)]
struct ResolveObjectRefResponse {
    url: String,
}

pub(super) async fn resolve_object_ref_url(
    client: &Client,
    config: &ObjectRefResolverConfig,
    object_ref: &str,
) -> Result<String, Status> {
    if object_ref.trim().is_empty() {
        return Err(Status::invalid_argument("media object ref is required"));
    }

    // The internal token is deliberately scoped to the resolver hop; the
    // returned URL is still validated by the download module before use.
    let response = client
        .post(&config.url)
        .header("X-MPP-Internal-Token", &config.internal_token)
        .json(&ResolveObjectRefRequest { object_ref })
        .send()
        .await
        .map_err(|_| Status::unavailable("failed to resolve media object ref"))?;
    let status = response.status();
    if !status.is_success() {
        if status == reqwest::StatusCode::UNAUTHORIZED || status == reqwest::StatusCode::FORBIDDEN {
            return Err(Status::failed_precondition(
                "media object ref resolver rejected internal token",
            ));
        }
        return Err(Status::unavailable(format!(
            "media object ref resolver returned HTTP {}",
            status.as_u16()
        )));
    }

    let resolved = response
        .json::<ResolveObjectRefResponse>()
        .await
        .map_err(|_| Status::unavailable("invalid media object ref resolver response"))?;
    if resolved.url.trim().is_empty() {
        return Err(Status::unavailable(
            "media object ref resolver returned an empty URL",
        ));
    }
    Ok(resolved.url)
}

#[cfg(test)]
mod tests {
    use std::sync::{Arc, Mutex};

    use axum::Router;
    use axum::body::Body;
    use axum::extract::Request;
    use axum::http::StatusCode;
    use axum::routing::post;

    use super::*;

    #[tokio::test]
    async fn resolves_object_ref_with_internal_token() {
        let seen = Arc::new(Mutex::new(Vec::<(String, String)>::new()));
        let seen_request = Arc::clone(&seen);
        let app = Router::new().route(
            "/internal/media/resolve",
            post(move |request: Request<Body>| {
                let seen_request = Arc::clone(&seen_request);
                async move {
                    let token = request
                        .headers()
                        .get("X-MPP-Internal-Token")
                        .and_then(|value| value.to_str().ok())
                        .unwrap_or_default()
                        .to_string();
                    let bytes = axum::body::to_bytes(request.into_body(), 1024)
                        .await
                        .expect("request body should be readable");
                    let body =
                        String::from_utf8(bytes.to_vec()).expect("request body should be utf8");
                    seen_request
                        .lock()
                        .expect("seen request mutex should not be poisoned")
                        .push((token, body));

                    (
                        StatusCode::OK,
                        [("content-type", "application/json")],
                        r#"{"url":"https://example.com/media.png","expires_at":"2026-06-06T08:00:00Z"}"#,
                    )
                }
            }),
        );
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0")
            .await
            .expect("test resolver should bind");
        let addr = listener
            .local_addr()
            .expect("test resolver address should be available");
        tokio::spawn(async move {
            axum::serve(listener, app)
                .await
                .expect("test resolver should serve");
        });
        let config = ObjectRefResolverConfig {
            url: format!("http://{addr}/internal/media/resolve"),
            internal_token: "test-internal-token".to_string(),
        };

        let resolved = resolve_object_ref_url(
            &Client::new(),
            &config,
            "mpp://media/11111111-1111-4111-8111-111111111111",
        )
        .await
        .expect("object ref should resolve");

        assert_eq!(resolved, "https://example.com/media.png");
        let seen = seen
            .lock()
            .expect("seen request mutex should not be poisoned");
        assert_eq!(seen.len(), 1);
        assert_eq!(seen[0].0, "test-internal-token");
        assert!(
            seen[0]
                .1
                .contains("mpp://media/11111111-1111-4111-8111-111111111111")
        );
    }
}
