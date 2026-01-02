// Package runtime provides the Runtime interface for job execution backends.
package runtime

import (
	"context"
	"fmt"
	"io"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
)

// DockerRuntime implements the Runtime interface using the Docker SDK.
type DockerRuntime struct {
	client *client.Client
}

// DockerHandle represents a running container.
type DockerHandle struct {
	client      *client.Client
	containerID string
}

func mapToEnvList(m map[string]string) []string {
	var env []string
	for k, v := range m {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	return env
}

// NewDockerRuntime creates a new Docker-based runtime.
func NewDockerRuntime() (*DockerRuntime, error) {
	// Initializes client from standard environment variables (DOCKER_HOST, etc.)
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("Failed to create Docker client: %w", err)
	}
	return &DockerRuntime{client: cli}, nil
}

// Start implements Runtime.Start using Docker containers.
func (d *DockerRuntime) Start(ctx context.Context, opts StartOptions) (Handle, error) {
	// Ensure Image Exists
	// Check if it exists locally first to save time.
	_, err := d.client.ImageInspect(ctx, opts.Image)
	if err != nil {
		// If image doesn't exist locally, pull it.
		reader, err := d.client.ImagePull(ctx, opts.Image, image.PullOptions{})
		if err != nil {
			return nil, fmt.Errorf("Failed to pull image %s: %w", opts.Image, err)
		}
		defer reader.Close()
		io.Copy(io.Discard, reader)
	}

	envVars := mapToEnvList(opts.Env)

	// Create Container
	containerConfig := &container.Config{
		Image: opts.Image,
		Cmd:   opts.Command,
		Env:   envVars,
		Tty:   true,
	}
	containerResponse, err := d.client.ContainerCreate(ctx, containerConfig, nil, nil, nil, "")
	if err != nil {
		return nil, fmt.Errorf("Failed to create container: %w", err)
	}

	if err := d.client.ContainerStart(ctx, containerResponse.ID, container.StartOptions{}); err != nil {
		return nil, fmt.Errorf("Failed to start container: %w", err)
	}

	return &DockerHandle{
		client:      d.client,
		containerID: containerResponse.ID,
	}, nil
}

func (h *DockerHandle) Wait(ctx context.Context) (ExitResult, error) {
	statusCh, errCh := h.client.ContainerWait(ctx, h.containerID, container.WaitConditionNotRunning)

	select {
	case err := <-errCh:
		return ExitResult{ExitCode: -1, Error: err}, err
	case status := <-statusCh:
		if status.Error != nil {
			return ExitResult{
					ExitCode: int(status.StatusCode),
					Error:    fmt.Errorf("%s", status.Error.Message),
				},
				nil
		}
		return ExitResult{ExitCode: int(status.StatusCode)}, nil
	case <-ctx.Done():
		return ExitResult{ExitCode: -1, Error: ctx.Err()}, ctx.Err()
	}
}

func (h *DockerHandle) Stop(ctx context.Context) error {
	timeOut := 5
	return h.client.ContainerStop(ctx, h.containerID, container.StopOptions{Timeout: &timeOut})
}

func (h *DockerHandle) StreamLogs(ctx context.Context) (io.ReadCloser, error) {
	return h.client.ContainerLogs(ctx, h.containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	})
}
