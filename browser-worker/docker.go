package main

import (
	"context"
	"fmt"
	"log"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

type DockerManager struct {
	cli *client.Client
}

func NewDockerManager() (*DockerManager, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionKind("1.41"))
	if err != nil {
		return nil, err
	}
	return &DockerManager{cli: cli}, nil
}

func (m *DockerManager) StartBrowserContainer(ctx context.Context, sessionID string) (containerID string, cdpPort, streamPort int, err error) {
	imageName := "mpp-browser-runtime"

	config := &container.Config{
		Image: imageName,
		ExposedPorts: nat.PortSet{
			"9222/tcp": {},
			"6080/tcp": {},
		},
		Env: []string{
			"RESOLUTION=1366x768x24",
		},
	}

	hostConfig := &container.HostConfig{
		PortBindings: nat.PortMap{
			"9222/tcp": []nat.PortBinding{{HostIP: "127.0.0.1", HostPort: "0"}}, // Random port on localhost
			"6080/tcp": []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: "0"}},   // Random port public (for stream)
		},
		// Limit resources for security and stability
		Resources: container.Resources{
			Memory:   1024 * 1024 * 1024, // 1GB
			NanoCPUs: 1000000000,         // 1 CPU
		},
	}

	resp, err := m.cli.ContainerCreate(ctx, config, hostConfig, nil, nil, "mpp-session-"+sessionID)
	if err != nil {
		return "", 0, 0, fmt.Errorf("failed to create container: %w", err)
	}

	if err := m.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", 0, 0, fmt.Errorf("failed to start container: %w", err)
	}

	// Inspect to get assigned ports
	json, err := m.cli.ContainerInspect(ctx, resp.ID)
	if err != nil {
		return "", 0, 0, fmt.Errorf("failed to inspect container: %w", err)
	}

	// Extract ports
	cdpPortStr := json.NetworkSettings.Ports["9222/tcp"][0].HostPort
	streamPortStr := json.NetworkSettings.Ports["6080/tcp"][0].HostPort

	fmt.Sscanf(cdpPortStr, "%d", &cdpPort)
	fmt.Sscanf(streamPortStr, "%d", &streamPort)

	log.Printf("Started container %s: CDP=%d, Stream=%d", resp.ID, cdpPort, streamPort)

	return resp.ID, cdpPort, streamPort, nil
}

func (m *DockerManager) StopContainer(ctx context.Context, id string) error {
	log.Printf("Stopping and removing container %s", id)
	
	// Stop with 10s timeout
	timeout := 10
	stopOptions := container.StopOptions{Timeout: &timeout}
	
	m.cli.ContainerStop(ctx, id, stopOptions)
	return m.cli.ContainerRemove(ctx, id, container.RemoveOptions{Force: true})
}
