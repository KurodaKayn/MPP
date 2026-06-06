package kubernetes

import (
	"context"
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	browserruntime "github.com/kurodakayn/mpp-browser-worker/internal/runtime"
)

func TestRuntimePodIncludesSessionMetadataAndResources(t *testing.T) {
	manager, err := NewManager(fake.NewSimpleClientset(), Config{
		Namespace:     "runtime-ns",
		RuntimeImage:  "registry.example/mpp-browser-runtime:sha",
		CPURequest:    "250m",
		CPULimit:      "750m",
		MemoryRequest: "256Mi",
		MemoryLimit:   "768Mi",
		ReadyTimeout:  time.Second,
	})
	require.NoError(t, err)
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	manager.now = func() time.Time { return now }

	pod := manager.runtimePod(browserruntime.StartSessionRequest{
		SessionID: "session-123",
		UserID:    "user-123",
		Platform:  "douyin",
		TTL:       30 * time.Minute,
	})

	require.Len(t, pod.Spec.Containers, 1)
	container := pod.Spec.Containers[0]
	assert.Equal(t, "mpp-browser-session-123", pod.Name)
	assert.Equal(t, "runtime-ns", pod.Namespace)
	assert.Equal(t, "session-123", pod.Labels["mpp.kurodakayn.dev/session-id"])
	assert.Equal(t, "fcdec6df4d44dbc6", pod.Labels["mpp.kurodakayn.dev/owner-hash"])
	assert.Equal(t, "douyin", pod.Labels["mpp.kurodakayn.dev/platform"])
	assert.Equal(t, "2026-06-06T12:30:00Z", pod.Annotations["mpp.kurodakayn.dev/expires-at"])
	require.NotNil(t, pod.Spec.ActiveDeadlineSeconds)
	assert.Equal(t, int64(1800), *pod.Spec.ActiveDeadlineSeconds)
	assert.Equal(t, "registry.example/mpp-browser-runtime:sha", container.Image)
	assert.Equal(t, "250m", container.Resources.Requests.Cpu().String())
	assert.Equal(t, "750m", container.Resources.Limits.Cpu().String())
	assert.Equal(t, "256Mi", container.Resources.Requests.Memory().String())
	assert.Equal(t, "768Mi", container.Resources.Limits.Memory().String())
	require.NotNil(t, container.ReadinessProbe)
	require.NotNil(t, container.ReadinessProbe.TCPSocket)
	assert.Equal(t, int32(cdpPort), container.ReadinessProbe.TCPSocket.Port.IntVal)
	require.NotNil(t, pod.Spec.AutomountServiceAccountToken)
	assert.False(t, *pod.Spec.AutomountServiceAccountToken)
	require.NotNil(t, container.SecurityContext)
	require.NotNil(t, container.SecurityContext.RunAsNonRoot)
	assert.True(t, *container.SecurityContext.RunAsNonRoot)
	require.NotNil(t, container.SecurityContext.RunAsUser)
	assert.Equal(t, runtimeUserID, *container.SecurityContext.RunAsUser)
	require.NotNil(t, container.SecurityContext.RunAsGroup)
	assert.Equal(t, runtimeGroupID, *container.SecurityContext.RunAsGroup)
	require.NotNil(t, container.SecurityContext.AllowPrivilegeEscalation)
	assert.False(t, *container.SecurityContext.AllowPrivilegeEscalation)
	require.NotNil(t, container.SecurityContext.Capabilities)
	assert.Contains(t, container.SecurityContext.Capabilities.Drop, corev1.Capability("ALL"))
	require.NotNil(t, container.SecurityContext.SeccompProfile)
	assert.Equal(t, corev1.SeccompProfileTypeRuntimeDefault, container.SecurityContext.SeccompProfile.Type)
}

func TestStartSessionCreatesReadyPodReference(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	manager, err := NewManager(client, Config{
		Namespace:    "runtime-ns",
		RuntimeImage: "mpp-browser-runtime",
		ReadyTimeout: 500 * time.Millisecond,
		PollInterval: time.Millisecond,
	})
	require.NoError(t, err)

	readyErrors := make(chan error, 1)
	go func() {
		readyErrors <- markPodReady(ctx, client, "runtime-ns", "mpp-browser-session-123", "10.42.0.7")
	}()

	reference, err := manager.StartSession(ctx, browserruntime.StartSessionRequest{
		SessionID: "session-123",
		UserID:    "user-123",
		Platform:  "zhihu",
		TTL:       time.Minute,
	})

	require.NoError(t, <-readyErrors)
	require.NoError(t, err)
	assert.Equal(t, browserruntime.DriverKubernetes, reference.Driver)
	assert.Equal(t, "mpp-browser-session-123", reference.RuntimeID)
	assert.Equal(t, "10.42.0.7", reference.CDPEndpoint.Host)
	assert.Equal(t, int(cdpPort), reference.CDPEndpoint.Port)
	assert.Equal(t, "10.42.0.7", reference.StreamEndpoint.Host)
	assert.Equal(t, int(streamPort), reference.StreamEndpoint.Port)
	assert.Equal(t, "session-123", reference.CleanupLabels["mpp.kurodakayn.dev/session-id"])
}

func TestStopSessionIgnoresMissingPod(t *testing.T) {
	manager, err := NewManager(fake.NewSimpleClientset(), Config{Namespace: "runtime-ns"})
	require.NoError(t, err)

	err = manager.StopSession(context.Background(), browserruntime.SessionReference{
		Driver:    browserruntime.DriverKubernetes,
		RuntimeID: "missing-pod",
	})

	require.NoError(t, err)
}

func TestReapExpiredSessionsDeletesOnlyExpiredRuntimePods(t *testing.T) {
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	client := fake.NewSimpleClientset(
		runtimePodFixture("expired", now.Add(-time.Minute)),
		runtimePodFixture("active", now.Add(time.Minute)),
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "missing-expiry",
				Namespace: "runtime-ns",
				Labels: map[string]string{
					appNameLabel:       appName,
					componentLabel:     browserRuntimeComponent,
					runtimeDriverLabel: browserruntime.DriverKubernetes,
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "other-component",
				Namespace: "runtime-ns",
				Labels: map[string]string{
					appNameLabel:       appName,
					componentLabel:     "backend",
					runtimeDriverLabel: browserruntime.DriverKubernetes,
				},
				Annotations: map[string]string{
					expiresAtAnnotation: now.Add(-time.Minute).Format(time.RFC3339),
				},
			},
		},
	)
	manager, err := NewManager(client, Config{Namespace: "runtime-ns"})
	require.NoError(t, err)
	manager.now = func() time.Time { return now }

	report, err := manager.ReapExpiredSessions(context.Background())

	require.NoError(t, err)
	assert.Equal(t, browserruntime.DriverKubernetes, report.Driver)
	assert.Equal(t, 1, report.DeletedSessions)
	assert.Equal(t, now.Add(-time.Minute), report.OldestExpiredAt)
	_, err = client.CoreV1().Pods("runtime-ns").Get(context.Background(), "expired", metav1.GetOptions{})
	assert.True(t, apierrors.IsNotFound(err))
	for _, name := range []string{"active", "missing-expiry", "other-component"} {
		_, err = client.CoreV1().Pods("runtime-ns").Get(context.Background(), name, metav1.GetOptions{})
		assert.NoError(t, err)
	}
}

func TestWaitForReadyPodRejectsTerminalPod(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "failed-pod", Namespace: "runtime-ns"},
		Status:     corev1.PodStatus{Phase: corev1.PodFailed},
	})
	manager, err := NewManager(client, Config{
		Namespace:    "runtime-ns",
		ReadyTimeout: 100 * time.Millisecond,
		PollInterval: time.Millisecond,
	})
	require.NoError(t, err)

	pod, err := manager.waitForReadyPod(ctx, "failed-pod")

	assert.Nil(t, pod)
	assert.ErrorContains(t, err, "terminal phase Failed")
}

func TestNewManagerRejectsInvalidResources(t *testing.T) {
	manager, err := NewManager(fake.NewSimpleClientset(), Config{CPURequest: "not-cpu"})

	assert.Nil(t, manager)
	assert.ErrorContains(t, err, "invalid kubernetes runtime resource cpu")
}

func runtimePodFixture(name string, expiresAt time.Time) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "runtime-ns",
			Labels: map[string]string{
				appNameLabel:       appName,
				componentLabel:     browserRuntimeComponent,
				runtimeDriverLabel: browserruntime.DriverKubernetes,
			},
			Annotations: map[string]string{
				expiresAtAnnotation: expiresAt.Format(time.RFC3339),
			},
		},
	}
}

func markPodReady(ctx context.Context, client *fake.Clientset, namespace string, name string, podIP string) error {
	deadline := time.After(300 * time.Millisecond)
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline:
			return fmt.Errorf("timed out waiting for pod creation")
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			pod, err := client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				continue
			}
			if err != nil {
				return err
			}
			pod.Status = corev1.PodStatus{
				Phase: corev1.PodRunning,
				PodIP: podIP,
				Conditions: []corev1.PodCondition{
					{Type: corev1.PodReady, Status: corev1.ConditionTrue},
				},
			}
			_, err = client.CoreV1().Pods(namespace).UpdateStatus(ctx, pod, metav1.UpdateOptions{})
			return err
		}
	}
}
