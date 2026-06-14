package main

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

type Suite struct {
	config   *Config
	kubectl  KubernetesClient
	reporter *Reporter
	http     HTTPDoer
}

func NewSuite(config *Config, kubectl KubernetesClient, reporter *Reporter, http HTTPDoer) *Suite {
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
