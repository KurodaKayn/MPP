use std::env;
use std::fmt;
use std::sync::Arc;

use content_pipeline_core::ProcessedAsset;
use object_store::aws::AmazonS3Builder;
use object_store::local::LocalFileSystem;
use object_store::path::Path as ObjectPath;
use object_store::{
    Attribute, Attributes, DynObjectStore, ObjectStore, ObjectStoreExt, PutOptions, PutPayload,
    TagSet,
};

const STORE_ENV: &str = "CONTENT_PIPELINE_MEDIA_OBJECT_STORE";
const ROOT_ENV: &str = "CONTENT_PIPELINE_MEDIA_OBJECT_ROOT";
const BUCKET_ENV: &str = "CONTENT_PIPELINE_MEDIA_OBJECT_BUCKET";
const ENDPOINT_ENV: &str = "CONTENT_PIPELINE_MEDIA_OBJECT_ENDPOINT";
const REGION_ENV: &str = "CONTENT_PIPELINE_MEDIA_OBJECT_REGION";
const ACCESS_KEY_ID_ENV: &str = "CONTENT_PIPELINE_MEDIA_OBJECT_ACCESS_KEY_ID";
const SECRET_ACCESS_KEY_ENV: &str = "CONTENT_PIPELINE_MEDIA_OBJECT_SECRET_ACCESS_KEY";
const ALLOW_HTTP_ENV: &str = "CONTENT_PIPELINE_MEDIA_OBJECT_ALLOW_HTTP";
const VIRTUAL_HOSTED_STYLE_ENV: &str = "CONTENT_PIPELINE_MEDIA_OBJECT_VIRTUAL_HOSTED_STYLE";
const PREFIX_ENV: &str = "CONTENT_PIPELINE_MEDIA_OBJECT_PREFIX";
const OBJECT_REF_PREFIX_ENV: &str = "CONTENT_PIPELINE_MEDIA_OBJECT_REF_PREFIX";
const RETENTION_DAYS_ENV: &str = "CONTENT_PIPELINE_MEDIA_OBJECT_RETENTION_DAYS";

const R2_BUCKET_ENV: &str = "R2_BUCKET";
const R2_ENDPOINT_ENV: &str = "R2_ENDPOINT";
const R2_REGION_ENV: &str = "R2_REGION";
const R2_ACCESS_KEY_ID_ENV: &str = "R2_ACCESS_KEY_ID";
const R2_SECRET_ACCESS_KEY_ENV: &str = "R2_SECRET_ACCESS_KEY";

const STORE_FILESYSTEM: &str = "filesystem";
const STORE_R2: &str = "r2";
const STORE_S3: &str = "s3";
const DEFAULT_OBJECT_PREFIX: &str = "content-pipeline/processed-media";
const DEFAULT_OBJECT_REF_PREFIX: &str = "mpp://content-pipeline/media/";
const DEFAULT_RETENTION_DAYS: u16 = 7;

#[derive(Clone)]
pub(crate) struct ProcessedMediaObjectStore {
    store: Arc<DynObjectStore>,
    key_prefix: String,
    object_ref_prefix: String,
    retention_days: u16,
}

impl fmt::Debug for ProcessedMediaObjectStore {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.debug_struct("ProcessedMediaObjectStore")
            .field("store", &self.store.to_string())
            .field("key_prefix", &self.key_prefix)
            .field("object_ref_prefix", &self.object_ref_prefix)
            .field("retention_days", &self.retention_days)
            .finish()
    }
}

impl ProcessedMediaObjectStore {
    pub(crate) fn from_env() -> Result<Self, MediaObjectStoreConfigError> {
        let provider = env_value(STORE_ENV).ok_or_else(|| {
            MediaObjectStoreConfigError(format!(
                "{STORE_ENV} is required because processed media responses use object_ref only"
            ))
        })?;

        let store: Arc<DynObjectStore> = match provider.to_ascii_lowercase().as_str() {
            STORE_FILESYSTEM => Arc::new(filesystem_store()?),
            STORE_R2 => Arc::new(r2_store()?),
            STORE_S3 => Arc::new(s3_store()?),
            _ => {
                return Err(MediaObjectStoreConfigError(format!(
                    "{STORE_ENV} must be one of {STORE_FILESYSTEM}, {STORE_R2}, or {STORE_S3}"
                )));
            }
        };

        Self::new(
            store,
            env_value(PREFIX_ENV).unwrap_or_else(|| DEFAULT_OBJECT_PREFIX.to_string()),
            env_value(OBJECT_REF_PREFIX_ENV)
                .unwrap_or_else(|| DEFAULT_OBJECT_REF_PREFIX.to_string()),
            parse_optional_u16(RETENTION_DAYS_ENV)?.unwrap_or(DEFAULT_RETENTION_DAYS),
        )
    }

    #[cfg(test)]
    pub(crate) fn test_store() -> Result<Self, MediaObjectStoreConfigError> {
        Self::new(
            Arc::new(object_store::memory::InMemory::new()),
            "processed".to_string(),
            "mpp://content-pipeline/media/".to_string(),
            DEFAULT_RETENTION_DAYS,
        )
    }

    pub(crate) fn new(
        store: Arc<DynObjectStore>,
        key_prefix: String,
        object_ref_prefix: String,
        retention_days: u16,
    ) -> Result<Self, MediaObjectStoreConfigError> {
        let key_prefix = normalize_key_prefix(&key_prefix);
        if key_prefix.is_empty() {
            return Err(MediaObjectStoreConfigError(format!(
                "{PREFIX_ENV} must not be empty"
            )));
        }

        let object_ref_prefix = normalize_object_ref_prefix(&object_ref_prefix);
        if object_ref_prefix.is_empty() {
            return Err(MediaObjectStoreConfigError(format!(
                "{OBJECT_REF_PREFIX_ENV} must not be empty"
            )));
        }
        if retention_days == 0 {
            return Err(MediaObjectStoreConfigError(format!(
                "{RETENTION_DAYS_ENV} must be greater than zero"
            )));
        }

        Ok(Self {
            store,
            key_prefix,
            object_ref_prefix,
            retention_days,
        })
    }

    pub(crate) async fn put_processed_asset(
        &self,
        processed: &ProcessedAsset,
    ) -> Result<StoredMediaObject, MediaObjectStoreError> {
        let key = self.object_key(processed)?;
        let location = ObjectPath::from(key.as_str());
        if let Ok(meta) = self.store.head(&location).await
            && meta.size == processed.byte_size
        {
            return Ok(StoredMediaObject {
                object_ref: self.object_ref(&key),
            });
        }

        let mut attributes = Attributes::new();
        attributes.insert(Attribute::ContentType, processed.mime_type.clone().into());
        attributes.insert(
            Attribute::Metadata("mpp-sha256".into()),
            processed.sha256.clone().into(),
        );
        attributes.insert(
            Attribute::Metadata("mpp-retention-days".into()),
            self.retention_days.to_string().into(),
        );

        let mut tags = TagSet::default();
        tags.push("mpp_media", "processed");
        tags.push("mpp_retention_days", &self.retention_days.to_string());

        let result = self
            .store
            .put_opts(
                &location,
                PutPayload::from(processed.bytes.clone()),
                PutOptions {
                    attributes,
                    tags,
                    ..PutOptions::default()
                },
            )
            .await;
        if matches!(result, Err(object_store::Error::NotImplemented { .. })) {
            self.store
                .put(&location, PutPayload::from(processed.bytes.clone()))
                .await
                .map_err(MediaObjectStoreError::Store)?;
        } else {
            result.map_err(MediaObjectStoreError::Store)?;
        }

        Ok(StoredMediaObject {
            object_ref: self.object_ref(&key),
        })
    }

    fn object_key(&self, processed: &ProcessedAsset) -> Result<String, MediaObjectStoreError> {
        let extension = extension_for_mime_type(&processed.mime_type).ok_or_else(|| {
            MediaObjectStoreError::UnsupportedMimeType(processed.mime_type.clone())
        })?;
        let shard = processed
            .sha256
            .get(..2)
            .ok_or(MediaObjectStoreError::InvalidSha256)?;
        Ok(format!(
            "{}/{}/{}.{}",
            self.key_prefix, shard, processed.sha256, extension
        ))
    }

    fn object_ref(&self, key: &str) -> String {
        format!("{}{}", self.object_ref_prefix, key)
    }
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct StoredMediaObject {
    pub(crate) object_ref: String,
}

#[derive(Debug)]
pub(crate) struct MediaObjectStoreConfigError(String);

impl fmt::Display for MediaObjectStoreConfigError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str(&self.0)
    }
}

impl std::error::Error for MediaObjectStoreConfigError {}

#[derive(Debug)]
pub(crate) enum MediaObjectStoreError {
    InvalidSha256,
    Store(object_store::Error),
    UnsupportedMimeType(String),
}

impl fmt::Display for MediaObjectStoreError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::InvalidSha256 => f.write_str("processed asset sha256 is invalid"),
            Self::Store(err) => write!(f, "object store error: {err}"),
            Self::UnsupportedMimeType(mime_type) => {
                write!(f, "unsupported stored media MIME type: {mime_type}")
            }
        }
    }
}

impl std::error::Error for MediaObjectStoreError {}

fn filesystem_store() -> Result<LocalFileSystem, MediaObjectStoreConfigError> {
    let root = env_value(ROOT_ENV).ok_or_else(|| {
        MediaObjectStoreConfigError(format!(
            "{ROOT_ENV} is required for filesystem media objects"
        ))
    })?;
    LocalFileSystem::new_with_prefix(root)
        .map_err(|err| MediaObjectStoreConfigError(format!("invalid {ROOT_ENV}: {err}")))
}

fn r2_store() -> Result<object_store::aws::AmazonS3, MediaObjectStoreConfigError> {
    let endpoint = env_value(ENDPOINT_ENV)
        .or_else(|| env_value(R2_ENDPOINT_ENV))
        .ok_or_else(|| {
            MediaObjectStoreConfigError(format!(
                "{ENDPOINT_ENV} or {R2_ENDPOINT_ENV} is required for R2 media objects"
            ))
        })?;
    let bucket = env_value(BUCKET_ENV)
        .or_else(|| env_value(R2_BUCKET_ENV))
        .ok_or_else(|| {
            MediaObjectStoreConfigError(format!(
                "{BUCKET_ENV} or {R2_BUCKET_ENV} is required for R2 media objects"
            ))
        })?;
    let access_key_id = env_value(ACCESS_KEY_ID_ENV)
        .or_else(|| env_value(R2_ACCESS_KEY_ID_ENV))
        .ok_or_else(|| {
            MediaObjectStoreConfigError(format!(
                "{ACCESS_KEY_ID_ENV} or {R2_ACCESS_KEY_ID_ENV} is required for R2 media objects"
            ))
        })?;
    let secret_access_key = env_value(SECRET_ACCESS_KEY_ENV)
        .or_else(|| env_value(R2_SECRET_ACCESS_KEY_ENV))
        .ok_or_else(|| {
            MediaObjectStoreConfigError(format!(
                "{SECRET_ACCESS_KEY_ENV} or {R2_SECRET_ACCESS_KEY_ENV} is required for R2 media objects"
            ))
        })?;
    let region = env_value(REGION_ENV)
        .or_else(|| env_value(R2_REGION_ENV))
        .unwrap_or_else(|| "auto".to_string());

    AmazonS3Builder::new()
        .with_endpoint(endpoint)
        .with_region(region)
        .with_bucket_name(bucket)
        .with_access_key_id(access_key_id)
        .with_secret_access_key(secret_access_key)
        .with_allow_http(parse_bool_env(ALLOW_HTTP_ENV, false))
        .with_virtual_hosted_style_request(parse_bool_env(VIRTUAL_HOSTED_STYLE_ENV, false))
        .build()
        .map_err(|err| MediaObjectStoreConfigError(format!("invalid R2 media object store: {err}")))
}

fn s3_store() -> Result<object_store::aws::AmazonS3, MediaObjectStoreConfigError> {
    let mut builder = AmazonS3Builder::from_env();
    if let Some(endpoint) = env_value(ENDPOINT_ENV) {
        builder = builder.with_endpoint(endpoint);
    }
    if let Some(region) = env_value(REGION_ENV) {
        builder = builder.with_region(region);
    }
    if let Some(bucket) = env_value(BUCKET_ENV) {
        builder = builder.with_bucket_name(bucket);
    }
    if let Some(access_key_id) = env_value(ACCESS_KEY_ID_ENV) {
        builder = builder.with_access_key_id(access_key_id);
    }
    if let Some(secret_access_key) = env_value(SECRET_ACCESS_KEY_ENV) {
        builder = builder.with_secret_access_key(secret_access_key);
    }
    builder = builder
        .with_allow_http(parse_bool_env(ALLOW_HTTP_ENV, false))
        .with_virtual_hosted_style_request(parse_bool_env(VIRTUAL_HOSTED_STYLE_ENV, false));

    builder
        .build()
        .map_err(|err| MediaObjectStoreConfigError(format!("invalid S3 media object store: {err}")))
}

fn env_value(key: &str) -> Option<String> {
    env::var(key)
        .ok()
        .map(|value| value.trim().to_string())
        .filter(|value| !value.is_empty())
}

fn parse_optional_u16(key: &str) -> Result<Option<u16>, MediaObjectStoreConfigError> {
    env_value(key)
        .map(|value| {
            value
                .parse::<u16>()
                .map_err(|_| MediaObjectStoreConfigError(format!("{key} must be an integer")))
        })
        .transpose()
}

fn parse_bool_env(key: &str, default: bool) -> bool {
    env_value(key).map_or(default, |value| {
        matches!(value.to_ascii_lowercase().as_str(), "1" | "true" | "yes")
    })
}

fn normalize_key_prefix(value: &str) -> String {
    value
        .trim()
        .trim_matches('/')
        .split('/')
        .filter(|part| !part.trim().is_empty())
        .collect::<Vec<_>>()
        .join("/")
}

fn normalize_object_ref_prefix(value: &str) -> String {
    let value = value.trim();
    if value.is_empty() {
        return String::new();
    }
    if value.ends_with('/') {
        value.to_string()
    } else {
        format!("{value}/")
    }
}

fn extension_for_mime_type(mime_type: &str) -> Option<&'static str> {
    match mime_type {
        "image/avif" => Some("avif"),
        "image/gif" => Some("gif"),
        "image/jpeg" => Some("jpg"),
        "image/png" => Some("png"),
        "image/webp" => Some("webp"),
        _ => None,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use object_store::local::LocalFileSystem;
    use object_store::path::Path as ObjectPath;
    use object_store::{ObjectStoreExt, PutPayload};
    use std::fs;

    fn processed_asset(bytes: Vec<u8>, sha256: &str) -> ProcessedAsset {
        ProcessedAsset {
            input_byte_size: bytes.len() as u64,
            byte_size: bytes.len() as u64,
            bytes,
            mime_type: "image/png".to_string(),
            width: 1,
            height: 1,
            sha256: sha256.to_string(),
            warnings: Vec::new(),
        }
    }

    #[tokio::test]
    async fn stores_processed_asset_under_hash_key() {
        let temp_dir = std::env::temp_dir().join(format!(
            "mpp-content-pipeline-media-store-{}",
            std::process::id()
        ));
        let _ = fs::remove_dir_all(&temp_dir);
        fs::create_dir_all(&temp_dir).expect("temp object root should be created");
        let store = Arc::new(
            LocalFileSystem::new_with_prefix(&temp_dir)
                .expect("local object store should initialize"),
        );
        let sink = ProcessedMediaObjectStore::new(
            store.clone(),
            "processed".to_string(),
            "mpp://content-pipeline/media".to_string(),
            7,
        )
        .expect("media object store should initialize");

        let stored = sink
            .put_processed_asset(&processed_asset(vec![1, 2, 3], "abcdef"))
            .await
            .expect("processed asset should be stored");

        assert_eq!(
            stored.object_ref,
            "mpp://content-pipeline/media/processed/ab/abcdef.png"
        );
        let data = store
            .get(&ObjectPath::from("processed/ab/abcdef.png"))
            .await
            .expect("stored object should exist")
            .bytes()
            .await
            .expect("stored object should be readable");
        assert_eq!(&data[..], &[1, 2, 3]);

        let _ = fs::remove_dir_all(temp_dir);
    }

    #[tokio::test]
    async fn skips_reupload_when_same_hash_and_size_exists() {
        let temp_dir = std::env::temp_dir().join(format!(
            "mpp-content-pipeline-media-dedup-{}",
            std::process::id()
        ));
        let _ = fs::remove_dir_all(&temp_dir);
        fs::create_dir_all(&temp_dir).expect("temp object root should be created");
        let store = Arc::new(
            LocalFileSystem::new_with_prefix(&temp_dir)
                .expect("local object store should initialize"),
        );
        store
            .put(
                &ObjectPath::from("processed/ab/abcdef.png"),
                PutPayload::from(vec![9, 9, 9]),
            )
            .await
            .expect("existing object should be stored");
        let sink = ProcessedMediaObjectStore::new(
            store.clone(),
            "processed".to_string(),
            "mpp://content-pipeline/media/".to_string(),
            7,
        )
        .expect("media object store should initialize");

        let stored = sink
            .put_processed_asset(&processed_asset(vec![1, 2, 3], "abcdef"))
            .await
            .expect("existing object should be reused");

        assert_eq!(
            stored.object_ref,
            "mpp://content-pipeline/media/processed/ab/abcdef.png"
        );
        let data = store
            .get(&ObjectPath::from("processed/ab/abcdef.png"))
            .await
            .expect("stored object should exist")
            .bytes()
            .await
            .expect("stored object should be readable");
        assert_eq!(&data[..], &[9, 9, 9]);

        let _ = fs::remove_dir_all(temp_dir);
    }

    #[test]
    fn normalizes_key_and_ref_prefixes() {
        assert_eq!(normalize_key_prefix("/tmp//processed/"), "tmp/processed");
        assert_eq!(
            normalize_object_ref_prefix("mpp://content-pipeline/media"),
            "mpp://content-pipeline/media/"
        );
    }
}
