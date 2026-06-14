package checks

import "net/http"

type Object map[string]any

type Response struct {
	Status  int
	Body    string
	Headers http.Header
}

type CheckFailure string

func (err CheckFailure) Error() string {
	return string(err)
}

type CheckSkip string

func (err CheckSkip) Error() string {
	return string(err)
}

type CheckResult struct {
	Name   string
	Detail string
}

type Settings struct {
	AppNamespace           string
	RuntimeNamespace       string
	ObservabilityNamespace string
	RolloutTimeout         int
	RequestTimeout         int
	CurlImage              string
	PublicURL              string
	APIBaseURL             string
	AuthToken              string
	ProjectID              string
	BrowserPlatform        string
	RunUserFlowProbes      bool
	RunBrowserSessionProbe bool
	RequireUserFlows       bool
	SkipPublic             bool
	SkipInternalHTTP       bool
	SkipRuntimeRBAC        bool
	SkipRuntimeCleanup     bool
	DryRun                 bool
	Verbose                bool
}

func (settings Settings) PublicURLConfigured() bool {
	return settings.PublicURL != ""
}

func (settings Settings) APIBaseURLConfigured() bool {
	return settings.APIBaseURL != ""
}

func (settings Settings) AuthConfigured() bool {
	return settings.AuthToken != ""
}

func (settings Settings) ProjectConfigured() bool {
	return settings.ProjectID != ""
}

func (settings Settings) UserFlowInputsConfigured() bool {
	return settings.APIBaseURLConfigured() && settings.AuthConfigured()
}

type KubernetesClient interface {
	CurrentContext() (string, error)
	ClientVersion() (any, error)
	Namespace(name string) (Object, error)
	Resource(kind string, name string, namespace string) (Object, error)
	ResourceList(kind string, namespace string, selector string) ([]Object, error)
	RolloutStatus(resource string, namespace string, timeout int) (string, error)
	AuthCanI(verb string, resource string, asUser string, namespace string) (string, error)
	Exec(resource string, command []string, namespace string, container string) (string, error)
	CurlFromEphemeralPod(namespace string, image string, targetURL string, timeout int, headers map[string]string, method string, body string) (string, error)
}

type HTTPDoer interface {
	Get(targetURL string, headers map[string]string) (Response, error)
	Post(targetURL string, headers map[string]string, jsonBody any) (Response, error)
	Delete(targetURL string, headers map[string]string) (Response, error)
}

type Reporter interface {
	Section(title string)
	Check(name string, required bool, fn func() (string, error))
	Skip(name string, detail string)
}

type Suite struct {
	config   Settings
	kubectl  KubernetesClient
	reporter Reporter
	http     HTTPDoer
}

func NewSuite(config Settings, kubectl KubernetesClient, reporter Reporter, http HTTPDoer) *Suite {
	return &Suite{
		config:   config,
		kubectl:  kubectl,
		reporter: reporter,
		http:     http,
	}
}

func (suite *Suite) Run() {
	suite.preflight()
	suite.clusterShape()
	suite.workloadRollouts()
	suite.serviceEndpoints()
	suite.configuration()
	suite.deploymentContracts()
	if !suite.config.SkipInternalHTTP {
		suite.internalReadiness()
	}
	if !suite.config.SkipRuntimeRBAC {
		suite.runtimeControls()
	}
	if !suite.config.SkipRuntimeCleanup {
		suite.runtimeCleanupState()
	}
	if !suite.config.SkipPublic {
		suite.publicGateway()
	}
	suite.authenticatedUserFlows()
}

func (suite *Suite) check(name string, required bool, fn func() (string, error)) {
	suite.reporter.Check(name, required, fn)
}
