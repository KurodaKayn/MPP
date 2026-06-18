package checks

var defaultDeployments = []string{
	"frontend",
	"backend",
	"publish-worker",
	"browser-worker",
	"ai-service",
	"content-pipeline-service",
	"collab-service",
}

var defaultServices = []string{
	"frontend",
	"backend",
	"browser-worker",
	"ai-service",
	"content-pipeline-service",
	"collab-service",
}

var requiredConfigKeys = []string{
	"BACKEND_API_BASE_URL",
	"BROWSER_WORKER_URL",
	"AI_SERVICE_URL",
	"CONTENT_PIPELINE_HOST",
	"CONTENT_PIPELINE_PORT",
	"CONTENT_PIPELINE_MEDIA_RESOLVER_URL",
	"CONTENT_PIPELINE_MEDIA_OBJECT_STORE",
	"CONTENT_PIPELINE_MEDIA_OBJECT_PREFIX",
	"CONTENT_PIPELINE_MEDIA_OBJECT_REF_PREFIX",
	"CONTENT_PIPELINE_MEDIA_OBJECT_RETENTION_DAYS",
	"COLLAB_INTERNAL_URL",
	"COLLAB_WEBSOCKET_URL_BASE",
	"DB_HOST",
	"DB_SSLMODE",
	"REDIS_ENDPOINT_MODE",
	"REDIS_ADDR",
	"REDIS_DB",
	"REDIS_TLS",
	"REDIS_TLS_CA_CERT",
	"REDIS_TLS_CA_FILE",
	"REDIS_TLS_SERVER_NAME",
	"REDIS_SENTINEL_ADDRS",
	"REDIS_SENTINEL_MASTER_NAME",
	"REDIS_POOL_SIZE",
	"REDIS_MIN_IDLE_CONNS",
	"REDIS_MAX_IDLE_CONNS",
	"REDIS_CONN_MAX_IDLE_TIME",
	"REDIS_CONN_MAX_LIFETIME",
	"OBJECT_STORAGE_PROVIDER",
	"R2_ACCOUNT_ID",
	"R2_BUCKET",
	"R2_ENDPOINT",
	"R2_REGION",
	"X_OAUTH2_CLIENT_ID",
	"X_OAUTH2_REDIRECT_URL",
	"X_OAUTH2_AUTHORIZE_URL",
	"X_OAUTH2_TOKEN_URL",
}

var requiredSecretKeys = []string{
	"JWT_SECRET",
	"DB_PASSWORD",
	"COLLAB_TOKEN_SECRET",
	"COOKIE_ENCRYPTION_KEY",
	"LLM_PROVIDER_KEY",
	"AI_SERVICE_INTERNAL_TOKEN",
	"BROWSER_WORKER_INTERNAL_TOKEN",
	"CONTENT_PIPELINE_INTERNAL_TOKEN",
	"R2_ACCESS_KEY_ID",
	"R2_SECRET_ACCESS_KEY",
	"X_OAUTH2_CLIENT_SECRET",
}

func DefaultDeploymentNames() []string {
	return cloneStrings(defaultDeployments)
}

func RequiredSecretKeyNames() []string {
	return cloneStrings(requiredSecretKeys)
}

func cloneStrings(values []string) []string {
	return append([]string(nil), values...)
}
