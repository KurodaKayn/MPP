package main

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

type DockerManager struct {
	cli *client.Client
}

func NewDockerManager() (*DockerManager, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.41"))
	if err != nil {
		return nil, err
	}
	return &DockerManager{cli: cli}, nil
}

func (m *DockerManager) StartBrowserContainer(ctx context.Context, sessionID string, adapterLoginURL string) (containerID string, containerIP string, cdpPort, streamPort int, err error) {
	imageName := "mpp-browser-runtime"

	config := &container.Config{
		Image: imageName,
		ExposedPorts: nat.PortSet{
			"9222/tcp": {},
			"6080/tcp": {},
		},
		Env: []string{
			"RESOLUTION=1366x768x24",
			"LOGIN_URL=" + adapterLoginURL,
		},
	}

	hostConfig := &container.HostConfig{
		PortBindings: nat.PortMap{
			"9222/tcp": []nat.PortBinding{{HostIP: "127.0.0.1", HostPort: "9222"}},
			"6080/tcp": []nat.PortBinding{{HostIP: "127.0.0.1", HostPort: "6080"}},
		},
		Resources: container.Resources{
			Memory:   1024 * 1024 * 1024,
			NanoCPUs: 1000000000,
		},
	}

	containerName := "mpp-session-" + sessionID

	// 1. Clean up ONLY the specific container for this session if it somehow exists
	m.cli.ContainerRemove(ctx, containerName, types.ContainerRemoveOptions{Force: true})

	resp, err := m.cli.ContainerCreate(ctx, config, hostConfig, nil, nil, containerName)
	if err != nil {
		return "", "", 0, 0, fmt.Errorf("failed to create container: %w", err)
	}

	if err := m.cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return "", "", 0, 0, fmt.Errorf("failed to start container: %w", err)
	}

	// Wait for services to start inside the container
	time.Sleep(5 * time.Second)

	// Since we use fixed ports, we return them directly
	return resp.ID, "127.0.0.1", 9222, 6080, nil
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

func (m *DockerManager) StopContainer(ctx context.Context, id string) error {
	log.Printf("Stopping and removing container %s", id)
	m.cli.ContainerStop(ctx, id, container.StopOptions{})
	return m.cli.ContainerRemove(ctx, id, types.ContainerRemoveOptions{Force: true})
}

// GetBrowserUUID reads the DevToolsActivePort file from the container to bypass HTTP Host checks
func (m *DockerManager) GetBrowserUUID(ctx context.Context, containerID string) (string, error) {
	reader, _, err := m.cli.CopyFromContainer(ctx, containerID, "/tmp/browser-profile/DevToolsActivePort")
	if err != nil {
		return "", err
	}
	defer reader.Close()

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
