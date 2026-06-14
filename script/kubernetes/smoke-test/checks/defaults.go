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
	"COLLAB_INTERNAL_URL",
	"COLLAB_WEBSOCKET_URL_BASE",
	"DB_HOST",
	"DB_SSLMODE",
	"REDIS_ADDR",
	"REDIS_TLS",
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
