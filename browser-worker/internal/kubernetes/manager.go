package kubernetes

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	browserruntime "github.com/kurodakayn/mpp-browser-worker/internal/runtime"
)

const (
	namespaceEnv    = "BROWSER_RUNTIME_KUBERNETES_NAMESPACE"
	kubeconfigEnv   = "BROWSER_RUNTIME_KUBERNETES_KUBECONFIG"
	readyTimeoutEnv = "BROWSER_RUNTIME_KUBERNETES_READY_TIMEOUT"

	imageEnv      = "BROWSER_RUNTIME_IMAGE"
	cpuRequestEnv = "BROWSER_RUNTIME_KUBERNETES_CPU_REQUEST"
	cpuLimitEnv   = "BROWSER_RUNTIME_KUBERNETES_CPU_LIMIT"
	memRequestEnv = "BROWSER_RUNTIME_KUBERNETES_MEMORY_REQUEST"
	memLimitEnv   = "BROWSER_RUNTIME_KUBERNETES_MEMORY_LIMIT"

	defaultNamespace    = "default"
	defaultRuntimeImage = "mpp-browser-runtime"
	defaultReadyTimeout = 60 * time.Second

	cdpPort    int32 = 9222
	streamPort int32 = 6080

	runtimeUserID  int64 = 1000
	runtimeGroupID int64 = 1000

	appNameLabel            = "app.kubernetes.io/name"
	componentLabel          = "app.kubernetes.io/component"
	runtimeDriverLabel      = "mpp.kurodakayn.dev/runtime-driver"
	sessionIDLabel          = "mpp.kurodakayn.dev/session-id"
	ownerHashLabel          = "mpp.kurodakayn.dev/owner-hash"
	platformLabel           = "mpp.kurodakayn.dev/platform"
	expiresAtAnnotation     = "mpp.kurodakayn.dev/expires-at"
	appName                 = "mpp"
	browserRuntimeComponent = "browser-runtime"
)

type Manager struct {
	client       kubeclient.Interface
	namespace    string
	runtimeImage string
	resources    corev1.ResourceRequirements
	readyTimeout time.Duration
	pollInterval time.Duration
	now          func() time.Time
}

func NewManagerFromEnv() (*Manager, error) {
	config, err := kubernetesConfigFromEnv()
	if err != nil {
		return nil, err
	}
	client, err := kubeclient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize kubernetes client: %w", err)
	}
	return NewManager(client, configFromEnv())
}

func NewManager(client kubeclient.Interface, config Config) (*Manager, error) {
	if client == nil {
		return nil, fmt.Errorf("kubernetes client is required")
	}
	config = config.withDefaults()
	resources, err := resourceRequirements(config)
	if err != nil {
		return nil, err
	}
	return &Manager{
		client:       client,
		namespace:    config.Namespace,
		runtimeImage: config.RuntimeImage,
		resources:    resources,
		readyTimeout: config.ReadyTimeout,
		pollInterval: config.PollInterval,
		now:          time.Now,
	}, nil
}

func (m *Manager) RuntimeDriver() string {
	return browserruntime.DriverKubernetes
}

type Config struct {
	Namespace     string
	RuntimeImage  string
	CPURequest    string
	CPULimit      string
	MemoryRequest string
	MemoryLimit   string
	ReadyTimeout  time.Duration
	PollInterval  time.Duration
}

func (c Config) withDefaults() Config {
	if strings.TrimSpace(c.Namespace) == "" {
		c.Namespace = defaultNamespace
	}
	if strings.TrimSpace(c.RuntimeImage) == "" {
		c.RuntimeImage = defaultRuntimeImage
	}
	if c.ReadyTimeout <= 0 {
		c.ReadyTimeout = defaultReadyTimeout
	}
	if c.PollInterval <= 0 {
		c.PollInterval = time.Second
	}
	if strings.TrimSpace(c.CPURequest) == "" {
		c.CPURequest = "500m"
	}
	if strings.TrimSpace(c.CPULimit) == "" {
		c.CPULimit = "1"
	}
	if strings.TrimSpace(c.MemoryRequest) == "" {
		c.MemoryRequest = "512Mi"
	}
	if strings.TrimSpace(c.MemoryLimit) == "" {
		c.MemoryLimit = "1Gi"
	}
	return c
}

func (m *Manager) StartSession(ctx context.Context, request browserruntime.StartSessionRequest) (browserruntime.SessionReference, error) {
	pod := m.runtimePod(request)
	if err := m.client.CoreV1().Pods(m.namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return browserruntime.SessionReference{}, fmt.Errorf("failed to remove existing runtime pod %s: %w", pod.Name, err)
	}
	created, err := m.client.CoreV1().Pods(m.namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return browserruntime.SessionReference{}, fmt.Errorf("failed to create runtime pod: %w", err)
	}
	readyPod, err := m.waitForReadyPod(ctx, created.Name)
	if err != nil {
		_ = m.StopSession(context.Background(), browserruntime.SessionReference{
			Driver:    browserruntime.DriverKubernetes,
			RuntimeID: created.Name,
		})
		return browserruntime.SessionReference{}, err
	}

	return browserruntime.SessionReference{
		Driver:    browserruntime.DriverKubernetes,
		RuntimeID: readyPod.Name,
		CDPEndpoint: browserruntime.Endpoint{
			Host: readyPod.Status.PodIP,
			Port: int(cdpPort),
		},
		StreamEndpoint: browserruntime.Endpoint{
			Host: readyPod.Status.PodIP,
			Port: int(streamPort),
		},
		CleanupLabels: cleanupLabels(request),
	}, nil
}

func (m *Manager) StopSession(ctx context.Context, reference browserruntime.SessionReference) error {
	if reference.RuntimeID == "" {
		return nil
	}
	err := m.client.CoreV1().Pods(m.namespace).Delete(ctx, reference.RuntimeID, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to delete runtime pod %s: %w", reference.RuntimeID, err)
	}
	return nil
}

func (m *Manager) ReapExpiredSessions(ctx context.Context) (browserruntime.ExpiredSessionReapReport, error) {
	report := browserruntime.ExpiredSessionReapReport{Driver: browserruntime.DriverKubernetes}
	pods, err := m.client.CoreV1().Pods(m.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: runtimePodSelector(),
	})
	if err != nil {
		return report, fmt.Errorf("failed to list runtime pods for cleanup: %w", err)
	}

	now := m.now()
	for _, pod := range pods.Items {
		expiresAt, ok := runtimePodExpiresAt(&pod)
		if !ok || expiresAt.After(now) {
			continue
		}
		if err := m.client.CoreV1().Pods(m.namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return report, fmt.Errorf("failed to delete expired runtime pod %s: %w", pod.Name, err)
		}
		report.DeletedSessions++
		if report.OldestExpiredAt.IsZero() || expiresAt.Before(report.OldestExpiredAt) {
			report.OldestExpiredAt = expiresAt
		}
	}
	return report, nil
}

func (m *Manager) runtimePod(request browserruntime.StartSessionRequest) *corev1.Pod {
	labels := cleanupLabels(request)
	labels[appNameLabel] = appName
	labels[componentLabel] = browserRuntimeComponent
	labels[runtimeDriverLabel] = browserruntime.DriverKubernetes

	now := m.now()
	expiresAt := now.Add(request.TTL)
	if request.TTL <= 0 {
		expiresAt = now.Add(15 * time.Minute)
	}
	activeDeadlineSeconds := int64(expiresAt.Sub(now).Seconds())
	if activeDeadlineSeconds <= 0 {
		activeDeadlineSeconds = int64((15 * time.Minute).Seconds())
	}

	runAsNonRoot := true
	allowPrivilegeEscalation := false
	automountServiceAccountToken := false
	runAsUser := runtimeUserID
	runAsGroup := runtimeGroupID
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        runtimePodName(request.SessionID),
			Namespace:   m.namespace,
			Labels:      labels,
			Annotations: map[string]string{expiresAtAnnotation: expiresAt.UTC().Format(time.RFC3339)},
		},
		Spec: corev1.PodSpec{
			AutomountServiceAccountToken: &automountServiceAccountToken,
			ActiveDeadlineSeconds:        &activeDeadlineSeconds,
			RestartPolicy:                corev1.RestartPolicyNever,
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot:   &runAsNonRoot,
				RunAsUser:      &runAsUser,
				RunAsGroup:     &runAsGroup,
				SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
			},
			Containers: []corev1.Container{
				{
					Name:  "browser-runtime",
					Image: m.runtimeImage,
					Env: []corev1.EnvVar{
						{Name: "RESOLUTION", Value: "1366x768x24"},
						{Name: "LOGIN_URL", Value: "about:blank"},
					},
					Ports: []corev1.ContainerPort{
						{Name: "cdp", ContainerPort: cdpPort},
						{Name: "stream", ContainerPort: streamPort},
					},
					Resources: m.resources,
					ReadinessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt32(cdpPort)},
						},
						InitialDelaySeconds: 2,
						PeriodSeconds:       2,
						TimeoutSeconds:      1,
						FailureThreshold:    30,
					},
					SecurityContext: &corev1.SecurityContext{
						RunAsNonRoot:             &runAsNonRoot,
						RunAsUser:                &runAsUser,
						RunAsGroup:               &runAsGroup,
						AllowPrivilegeEscalation: &allowPrivilegeEscalation,
						Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
						SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
					},
				},
			},
		},
	}
}

func (m *Manager) waitForReadyPod(ctx context.Context, name string) (*corev1.Pod, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, m.readyTimeout)
	defer cancel()

	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	for {
		pod, err := m.client.CoreV1().Pods(m.namespace).Get(timeoutCtx, name, metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to inspect runtime pod %s: %w", name, err)
		}
		if err == nil {
			if podReady(pod) && pod.Status.PodIP != "" {
				return pod, nil
			}
			if pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded {
				return nil, fmt.Errorf("runtime pod %s reached terminal phase %s before readiness", name, pod.Status.Phase)
			}
		}

		select {
		case <-timeoutCtx.Done():
			return nil, fmt.Errorf("runtime pod %s did not become ready before timeout: %w", name, timeoutCtx.Err())
		case <-ticker.C:
		}
	}
}

func podReady(pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func cleanupLabels(request browserruntime.StartSessionRequest) map[string]string {
	return map[string]string{
		sessionIDLabel: request.SessionID,
		ownerHashLabel: ownerHash(request.UserID),
		platformLabel:  sanitizeLabelValue(request.Platform),
	}
}

func runtimePodSelector() string {
	return labels.SelectorFromSet(labels.Set{
		appNameLabel:       appName,
		componentLabel:     browserRuntimeComponent,
		runtimeDriverLabel: browserruntime.DriverKubernetes,
	}).String()
}

func runtimePodExpiresAt(pod *corev1.Pod) (time.Time, bool) {
	if pod == nil || pod.Annotations == nil {
		return time.Time{}, false
	}
	raw := strings.TrimSpace(pod.Annotations[expiresAtAnnotation])
	if raw == "" {
		return time.Time{}, false
	}
	expiresAt, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, false
	}
	return expiresAt, true
}

func runtimePodName(sessionID string) string {
	name := sanitizeName(sessionID)
	if name == "" {
		name = "session"
	}
	return "mpp-browser-" + name
}

func ownerHash(ownerID string) string {
	sum := sha256.Sum256([]byte(ownerID))
	return hex.EncodeToString(sum[:])[:16]
}

func sanitizeName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func sanitizeLabelValue(value string) string {
	value = sanitizeName(value)
	if value == "" {
		return "unknown"
	}
	if len(value) > 63 {
		return strings.Trim(value[:63], "-")
	}
	return value
}

func resourceRequirements(config Config) (corev1.ResourceRequirements, error) {
	requests, err := resourceList(map[corev1.ResourceName]string{
		corev1.ResourceCPU:    config.CPURequest,
		corev1.ResourceMemory: config.MemoryRequest,
	})
	if err != nil {
		return corev1.ResourceRequirements{}, err
	}
	limits, err := resourceList(map[corev1.ResourceName]string{
		corev1.ResourceCPU:    config.CPULimit,
		corev1.ResourceMemory: config.MemoryLimit,
	})
	if err != nil {
		return corev1.ResourceRequirements{}, err
	}
	return corev1.ResourceRequirements{Requests: requests, Limits: limits}, nil
}

func resourceList(values map[corev1.ResourceName]string) (corev1.ResourceList, error) {
	result := corev1.ResourceList{}
	for name, raw := range values {
		quantity, err := resource.ParseQuantity(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid kubernetes runtime resource %s %q: %w", name, raw, err)
		}
		result[name] = quantity
	}
	return result, nil
}

func configFromEnv() Config {
	return Config{
		Namespace:     envOrDefault(namespaceEnv, defaultNamespace),
		RuntimeImage:  envOrDefault(imageEnv, defaultRuntimeImage),
		CPURequest:    envOrDefault(cpuRequestEnv, "500m"),
		CPULimit:      envOrDefault(cpuLimitEnv, "1"),
		MemoryRequest: envOrDefault(memRequestEnv, "512Mi"),
		MemoryLimit:   envOrDefault(memLimitEnv, "1Gi"),
		ReadyTimeout:  durationEnvOrDefault(readyTimeoutEnv, defaultReadyTimeout),
		PollInterval:  time.Second,
	}
}

func kubernetesConfigFromEnv() (*rest.Config, error) {
	if kubeconfig := strings.TrimSpace(os.Getenv(kubeconfigEnv)); kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	config, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		return nil, fmt.Errorf("failed to load kubernetes config: %w", err)
	}
	return config, nil
}

func envOrDefault(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func durationEnvOrDefault(name string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err == nil {
		return parsed
	}
	seconds, err := strconv.Atoi(value)
	if err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return fallback
}
