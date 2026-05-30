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

func (m *DockerManager) StartBrowserContainer(ctx context.Context, sessionID string) (containerID string, containerIP string, cdpPort, streamPort int, err error) {
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
		PublishAllPorts: true,
		Resources: container.Resources{
			Memory:   1024 * 1024 * 1024,
			NanoCPUs: 1000000000,
		},
	}

	containerName := "mpp-session-" + sessionID
	m.cli.ContainerRemove(ctx, containerName, types.ContainerRemoveOptions{Force: true})

	resp, err := m.cli.ContainerCreate(ctx, config, hostConfig, nil, nil, containerName)
	if err != nil {
		return "", "", 0, 0, fmt.Errorf("failed to create container: %w", err)
	}

	if err := m.cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return "", "", 0, 0, fmt.Errorf("failed to start container: %w", err)
	}

	var ports nat.PortMap
	var inspectErr error
	var containerIPAddr string

	for i := 0; i < 10; i++ {
		time.Sleep(1 * time.Second)
		json, err := m.cli.ContainerInspect(ctx, resp.ID)
		if err != nil {
			inspectErr = err
			continue
		}

		ports = json.NetworkSettings.Ports
		if len(ports["9222/tcp"]) > 0 && len(ports["6080/tcp"]) > 0 {
			containerIPAddr = json.NetworkSettings.IPAddress
			if containerIPAddr == "" {
				for _, net := range json.NetworkSettings.Networks {
					containerIPAddr = net.IPAddress
					break
				}
			}
			inspectErr = nil
			break
		}
		inspectErr = fmt.Errorf("ports not yet assigned by docker (attempt %d/10)", i+1)
	}

	if inspectErr != nil {
		logBody, _ := m.cli.ContainerLogs(ctx, resp.ID, types.ContainerLogsOptions{ShowStdout: true, ShowStderr: true, Tail: "20"})
		containerLogs := "could not retrieve logs"
		if logBody != nil {
			defer logBody.Close()
			content, _ := io.ReadAll(logBody)
			containerLogs = string(content)
		}
		return "", "", 0, 0, fmt.Errorf("%v. Logs: %s", inspectErr, containerLogs)
	}

	cdpPortStr := ports["9222/tcp"][0].HostPort
	streamPortStr := ports["6080/tcp"][0].HostPort

	fmt.Sscanf(cdpPortStr, "%d", &cdpPort)
	fmt.Sscanf(streamPortStr, "%d", &streamPort)

	log.Printf("Started container %s with RANDOM PORTS: CDP=%d, Stream=%d", resp.ID[:12], cdpPort, streamPort)

	return resp.ID, containerIPAddr, cdpPort, streamPort, nil
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
