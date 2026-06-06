package container

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"

	browserruntime "github.com/kurodakayn/mpp-browser-worker/internal/runtime"
)

type Manager struct {
	cli            *client.Client
	runtimeImage   string
	runtimeNetwork string
	runtimeBindIP  string
	runtimeHost    string
}

func NewManager() (*Manager, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.41"))
	if err != nil {
		return nil, err
	}
	return &Manager{
		cli:            cli,
		runtimeImage:   envOrDefault("BROWSER_RUNTIME_IMAGE", "mpp-browser-runtime"),
		runtimeNetwork: strings.TrimSpace(os.Getenv("BROWSER_RUNTIME_NETWORK")),
		runtimeBindIP:  envOrDefault("BROWSER_RUNTIME_BIND_IP", "127.0.0.1"),
		runtimeHost:    envOrDefault("BROWSER_RUNTIME_HOST", "127.0.0.1"),
	}, nil
}

func (m *Manager) RuntimeDriver() string {
	return browserruntime.DriverDocker
}

func (m *Manager) StartSession(ctx context.Context, request browserruntime.StartSessionRequest) (browserruntime.SessionReference, error) {
	containerID, containerHost, cdpPort, streamPort, err := m.startBrowserContainer(ctx, request.SessionID)
	if err != nil {
		return browserruntime.SessionReference{}, err
	}
	return browserruntime.SessionReference{
		Driver:    browserruntime.DriverDocker,
		RuntimeID: containerID,
		CDPEndpoint: browserruntime.Endpoint{
			Host: containerHost,
			Port: cdpPort,
		},
		StreamEndpoint: browserruntime.Endpoint{
			Host: containerHost,
			Port: streamPort,
		},
		CleanupLabels: map[string]string{
			"session_id": request.SessionID,
			"user_id":    request.UserID,
			"platform":   request.Platform,
		},
	}, nil
}

func (m *Manager) StopSession(ctx context.Context, reference browserruntime.SessionReference) error {
	if reference.RuntimeID == "" {
		return nil
	}
	return m.stopContainer(ctx, reference.RuntimeID)
}

func (m *Manager) StartBrowserContainer(ctx context.Context, sessionID string) (containerID string, containerIP string, cdpPort, streamPort int, err error) {
	return m.startBrowserContainer(ctx, sessionID)
}

func (m *Manager) startBrowserContainer(ctx context.Context, sessionID string) (containerID string, containerIP string, cdpPort, streamPort int, err error) {
	config := &dockercontainer.Config{
		Image: m.runtimeImage,
		ExposedPorts: nat.PortSet{
			"9222/tcp": {},
			"6080/tcp": {},
		},
		Env: []string{
			"RESOLUTION=1366x768x24",
			"LOGIN_URL=about:blank",
		},
	}

	hostConfig := &dockercontainer.HostConfig{
		Resources: dockercontainer.Resources{
			Memory:   1024 * 1024 * 1024,
			NanoCPUs: 1000000000,
		},
	}
	networkingConfig := &network.NetworkingConfig{}
	if m.runtimeNetwork != "" {
		hostConfig.NetworkMode = dockercontainer.NetworkMode(m.runtimeNetwork)
		networkingConfig.EndpointsConfig = map[string]*network.EndpointSettings{
			m.runtimeNetwork: {},
		}
	} else {
		hostConfig.PortBindings = nat.PortMap{
			"9222/tcp": []nat.PortBinding{{HostIP: m.runtimeBindIP}},
			"6080/tcp": []nat.PortBinding{{HostIP: m.runtimeBindIP}},
		}
	}

	containerName := "mpp-session-" + sessionID

	// Clean up only the specific container for this session if it somehow exists.
	_ = m.cli.ContainerRemove(ctx, containerName, types.ContainerRemoveOptions{Force: true})

	resp, err := m.cli.ContainerCreate(ctx, config, hostConfig, networkingConfig, nil, containerName)
	if err != nil {
		return "", "", 0, 0, fmt.Errorf("failed to create container: %w", err)
	}

	if err := m.cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return "", "", 0, 0, fmt.Errorf("failed to start container: %w", err)
	}

	// Wait for services to start inside the container
	time.Sleep(5 * time.Second)

	inspect, err := m.cli.ContainerInspect(ctx, resp.ID)
	if err != nil {
		_ = m.StopContainer(context.Background(), resp.ID)
		return "", "", 0, 0, fmt.Errorf("failed to inspect container ports: %w", err)
	}
	if m.runtimeNetwork != "" {
		containerHost, err := containerNetworkIP(inspect.NetworkSettings.Networks, m.runtimeNetwork)
		if err != nil {
			_ = m.StopContainer(context.Background(), resp.ID)
			return "", "", 0, 0, err
		}
		return resp.ID, containerHost, 9222, 6080, nil
	}
	cdpPort, err = mappedHostPort(inspect.NetworkSettings.Ports, "9222/tcp")
	if err != nil {
		_ = m.StopContainer(context.Background(), resp.ID)
		return "", "", 0, 0, err
	}
	streamPort, err = mappedHostPort(inspect.NetworkSettings.Ports, "6080/tcp")
	if err != nil {
		_ = m.StopContainer(context.Background(), resp.ID)
		return "", "", 0, 0, err
	}

	return resp.ID, m.runtimeHost, cdpPort, streamPort, nil
}

func containerNetworkIP(networks map[string]*network.EndpointSettings, networkName string) (string, error) {
	endpoint, ok := networks[networkName]
	if !ok || endpoint == nil || endpoint.IPAddress == "" {
		return "", fmt.Errorf("container is not attached to runtime network %q", networkName)
	}
	return endpoint.IPAddress, nil
}

func mappedHostPort(ports nat.PortMap, port string) (int, error) {
	bindings := ports[nat.Port(port)]
	if len(bindings) == 0 || bindings[0].HostPort == "" {
		return 0, fmt.Errorf("container port %s is not mapped", port)
	}
	var hostPort int
	if _, err := fmt.Sscanf(bindings[0].HostPort, "%d", &hostPort); err != nil {
		return 0, fmt.Errorf("invalid mapped port %q: %w", bindings[0].HostPort, err)
	}
	return hostPort, nil
}

func (m *Manager) StopContainer(ctx context.Context, id string) error {
	return m.stopContainer(ctx, id)
}

func (m *Manager) stopContainer(ctx context.Context, id string) error {
	log.Printf("Stopping and removing container %s", id)
	if err := m.cli.ContainerStop(ctx, id, dockercontainer.StopOptions{}); err != nil {
		log.Printf("Failed to stop container %s: %v", id, err)
	}
	return m.cli.ContainerRemove(ctx, id, types.ContainerRemoveOptions{Force: true})
}

// GetBrowserUUID reads the DevToolsActivePort file from the container to bypass HTTP Host checks
func (m *Manager) GetBrowserUUID(ctx context.Context, containerID string) (string, error) {
	reader, _, err := m.cli.CopyFromContainer(ctx, containerID, "/tmp/browser-profile/DevToolsActivePort")
	if err != nil {
		return "", err
	}
	defer func() { _ = reader.Close() }()

	tr := tar.NewReader(reader)
	_, err = tr.Next() // Get to the first file
	if err != nil {
		return "", err
	}

	content, err := io.ReadAll(tr)
	if err != nil {
		return "", err
	}

	lines := strings.Split(string(content), "\n")
	if len(lines) >= 2 {
		return strings.TrimSpace(lines[1]), nil
	}
	return "", fmt.Errorf("invalid DevToolsActivePort format")
}

func envOrDefault(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}
