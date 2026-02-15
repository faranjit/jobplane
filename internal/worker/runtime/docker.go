// Package runtime provides the Runtime interface for job execution backends.
package runtime

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
)

const (
	containerOutputDir = "/jobplane/output"
)

// DockerRuntime implements the Runtime interface using the Docker SDK.
type DockerRuntime struct {
	client *client.Client
}

// DockerHandle represents a running container.
type DockerHandle struct {
	client      *client.Client
	containerID string
	// hostWorkDir is the temporary directory on the Worker where it extracts results
	hostWorkDir string
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
	// Create a temporary directory on the Worker to hold the results later
	hostWorkDir, err := os.MkdirTemp("", "jobplane-docker-*")
	if err != nil {
		return nil, fmt.Errorf("Failed to create host work directory: %w", err)
	}

	// Ensure Image Exists
	// Check if it exists locally first to save time.
	_, err = d.client.ImageInspect(ctx, opts.Image)
	if err != nil {
		// If image doesn't exist locally, pull it.
		reader, err := d.client.ImagePull(ctx, opts.Image, image.PullOptions{})
		if err != nil {
			os.RemoveAll(hostWorkDir)
			return nil, fmt.Errorf("Failed to pull image %s: %w", opts.Image, err)
		}
		defer reader.Close()
		io.Copy(io.Discard, reader)
	}

	opts.Env["JOBPLANE_OUTPUT_DIR"] = containerOutputDir

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
		os.RemoveAll(hostWorkDir)
		return nil, fmt.Errorf("Failed to create container: %w", err)
	}

	if err := d.client.ContainerStart(ctx, containerResponse.ID, container.StartOptions{}); err != nil {
		os.RemoveAll(hostWorkDir)
		return nil, fmt.Errorf("Failed to start container: %w", err)
	}

	return &DockerHandle{
		client:      d.client,
		containerID: containerResponse.ID,
		hostWorkDir: hostWorkDir,
	}, nil
}

func (h *DockerHandle) Wait(ctx context.Context) (ExitResult, error) {
	statusCh, errCh := h.client.ContainerWait(ctx, h.containerID, container.WaitConditionNotRunning)

	select {
	case err := <-errCh:
		return ExitResult{ExitCode: -1, Error: err}, err
	case status := <-statusCh:
		result := ExitResult{ExitCode: int(status.StatusCode)}
		if status.Error != nil {
			result.Error = fmt.Errorf("%s", status.Error.Message)
		}

		copyCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := h.copyResultsFromContainer(copyCtx); err != nil {
			// Log this, but don't necessarily fail the job execution status
			// usually, if the folder doesn't exist, it means the execution didn't write anything.
			log.Printf("Failed to copy results from container: %v", err)
		}

		removeCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = h.client.ContainerRemove(removeCtx, h.containerID, container.RemoveOptions{Force: true})

		return result, nil
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

// ResultDir returns the directory where the result of this job is stored.
func (h *DockerHandle) ResultDir() string {
	return h.hostWorkDir
}

// Cleanup removes the temporary directory where the result of this job is stored.
// This is called by the worker after the job is completed.
func (h *DockerHandle) Cleanup() error {
	if h.hostWorkDir != "" {
		return os.RemoveAll(h.hostWorkDir)
	}
	return nil
}

func (h *DockerHandle) copyResultsFromContainer(ctx context.Context) error {
	const srcPath = containerOutputDir + "/."
	content, _, err := h.client.CopyFromContainer(ctx, h.containerID, srcPath)
	if err != nil {
		// If the directory doesn't exist, it usually means the execution didn't write anything.
		if errdefs.IsNotFound(err) {
			return nil
		}
		return err
	}
	defer content.Close()

	return untar(h.hostWorkDir, content)
}

func untar(dest string, r io.Reader) error {
	tr := tar.NewReader(r)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// sanitize path — skip root directory entry from docker cp
		if header.Name == "./" || header.Name == "." {
			continue
		}
		target := filepath.Join(dest, header.Name)
		if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			// Ensure parent dir exists
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			f, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
		}
	}
	return nil
}
