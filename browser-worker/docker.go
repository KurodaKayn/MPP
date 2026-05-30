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
	
	fixedCDPPort := 9222
	fixedStreamPort := 6080

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
			"9222/tcp": []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: fmt.Sprintf("%d", fixedCDPPort)}}, 
			"6080/tcp": []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: fmt.Sprintf("%d", fixedStreamPort)}},
		},
		Resources: container.Resources{
			Memory:   1024 * 1024 * 1024,
			NanoCPUs: 1000000000,
		},
	}

	// Important: We must remove any existing container using these ports first
	// We'll search for any container with our prefix or using our ports
	containerName := "mpp-session-" + sessionID
	
	// Clean up previous attempts to avoid "port already allocated"
	m.cli.ContainerRemove(ctx, containerName, types.ContainerRemoveOptions{Force: true})
	
	// Also clean up any other container that might be holding the ports
	containers, _ := m.cli.ContainerList(ctx, types.ContainerListOptions{All: true})
	for _, c := range containers {
		for _, name := range c.Names {
			if strings.Contains(name, "mpp-session") {
				m.cli.ContainerRemove(ctx, c.ID, types.ContainerRemoveOptions{Force: true})
				break
			}
		}
	}

	resp, err := m.cli.ContainerCreate(ctx, config, hostConfig, nil, nil, containerName)
	if err != nil {
		return "", "", 0, 0, fmt.Errorf("failed to create container: %w", err)
	}

	if err := m.cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return "", "", 0, 0, fmt.Errorf("failed to start container: %w", err)
	}

	// Fixed ports don't need a retry loop for discovery, but we wait for health
	time.Sleep(5 * time.Second)

	json, err := m.cli.ContainerInspect(ctx, resp.ID)
	if err != nil {
		return "", "", 0, 0, fmt.Errorf("failed to inspect container: %w", err)
	}

	containerIPAddr := json.NetworkSettings.IPAddress
	if containerIPAddr == "" {
		for _, net := range json.NetworkSettings.Networks {
			containerIPAddr = net.IPAddress
			break
		}
	}

	log.Printf("Started container %s with FIXED PORTS: CDP=%d, Stream=%d", resp.ID[:12], fixedCDPPort, fixedStreamPort)

	return resp.ID, containerIPAddr, fixedCDPPort, fixedStreamPort, nil
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
