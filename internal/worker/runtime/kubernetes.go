// Package runtime provides the Runtime interface for job execution backends.
package runtime

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// KubernetesConfig holds configuration for the Kubernetes runtime.
type KubernetesConfig struct {
	// Namespace where jobs will be created
	Namespace string
	// ServiceAccount for job pods (optional)
	ServiceAccount string
	// Default resource limits for jobs
	DefaultCPULimit    string
	DefaultMemoryLimit string
}

// KubernetesRuntime implements the Runtime interface using Kubernetes Jobs.
type KubernetesRuntime struct {
	clientset kubernetes.Interface
	config    KubernetesConfig
}

// KubernetesHandle represents a running Kubernetes Job.
type KubernetesHandle struct {
	clientset kubernetes.Interface
	namespace string
	jobName   string
	podName   string // Populated after pod starts
}

// homeDir returns the user's home directory.
func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // Windows
}

// NewKubernetesRuntime creates a new Kubernetes-based runtime.
// Tries in-cluster configuration first, falls back to kubeconfig for local development.
func NewKubernetesRuntime(cfg KubernetesConfig) (*KubernetesRuntime, error) {
	// Try in-cluster config first
	config, err := rest.InClusterConfig()
	if err != nil {
		// Fall back to kubeconfig for local development
		log.Printf("In-cluster config not available, trying kubeconfig: %v", err)
		kubeconfig := filepath.Join(homeDir(), ".kube", "config")
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("failed to build kubernetes config: %w", err)
		}
		log.Printf("Using kubeconfig: %s", kubeconfig)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes clientset: %w", err)
	}

	// Set defaults
	if cfg.Namespace == "" {
		cfg.Namespace = "default"
	}
	if cfg.DefaultCPULimit == "" {
		cfg.DefaultCPULimit = "500m"
	}
	if cfg.DefaultMemoryLimit == "" {
		cfg.DefaultMemoryLimit = "256Mi"
	}

	return &KubernetesRuntime{
		clientset: clientset,
		config:    cfg,
	}, nil
}

// Start implements Runtime.Start by creating a Kubernetes Job.
func (k *KubernetesRuntime) Start(ctx context.Context, opts StartOptions) (Handle, error) {
	// Generate unique job name
	jobName := fmt.Sprintf("jobplane-%d", time.Now().UnixNano())

	// Build environment variables
	var envVars []corev1.EnvVar
	for key, value := range opts.Env {
		envVars = append(envVars, corev1.EnvVar{Name: key, Value: value})
	}

	// Build resource limits
	resources := corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(k.config.DefaultCPULimit),
			corev1.ResourceMemory: resource.MustParse(k.config.DefaultMemoryLimit),
		},
	}

	// Create Job spec
	backoffLimit := int32(0) // No retries - jobplane handles retries
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: k.config.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "jobplane",
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoffLimit,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"job-name":                     jobName,
						"app.kubernetes.io/managed-by": "jobplane",
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:      "job",
							Image:     opts.Image,
							Command:   opts.Command,
							Env:       envVars,
							Resources: resources,
						},
					},
				},
			},
		},
	}

	// Set service account if configured
	if k.config.ServiceAccount != "" {
		job.Spec.Template.Spec.ServiceAccountName = k.config.ServiceAccount
	}

	// Create the Job
	createdJob, err := k.clientset.BatchV1().Jobs(k.config.Namespace).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes job: %w", err)
	}

	log.Printf("Created Kubernetes Job %s in namespace %s", createdJob.Name, k.config.Namespace)

	return &KubernetesHandle{
		clientset: k.clientset,
		namespace: k.config.Namespace,
		jobName:   createdJob.Name,
	}, nil
}

// Wait blocks until the job's pod completes and returns the result.
func (h *KubernetesHandle) Wait(ctx context.Context) (ExitResult, error) {
	// First, wait for pod to be created and get its name
	podName, err := h.waitForPod(ctx)
	if err != nil {
		return ExitResult{ExitCode: -1, Error: err}, err
	}
	h.podName = podName

	// Watch pod until it completes
	watcher, err := h.clientset.CoreV1().Pods(h.namespace).Watch(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("metadata.name=%s", podName),
	})
	if err != nil {
		return ExitResult{ExitCode: -1, Error: err}, err
	}
	defer watcher.Stop()

	for event := range watcher.ResultChan() {
		if event.Type == watch.Error {
			return ExitResult{ExitCode: -1, Error: fmt.Errorf("watch error")}, fmt.Errorf("watch error")
		}

		pod, ok := event.Object.(*corev1.Pod)
		if !ok {
			continue
		}

		switch pod.Status.Phase {
		case corev1.PodSucceeded:
			return ExitResult{ExitCode: 0}, nil

		case corev1.PodFailed:
			exitCode := -1
			var errMsg error
			if len(pod.Status.ContainerStatuses) > 0 {
				cs := pod.Status.ContainerStatuses[0]
				if cs.State.Terminated != nil {
					exitCode = int(cs.State.Terminated.ExitCode)
					if cs.State.Terminated.Reason != "" {
						errMsg = fmt.Errorf("%s", cs.State.Terminated.Reason)
					}
				}
			}
			return ExitResult{ExitCode: exitCode, Error: errMsg}, nil
		}
	}

	// Context cancelled
	return ExitResult{ExitCode: -1, Error: ctx.Err()}, ctx.Err()
}

// waitForPod waits for the job's pod to be created and returns its name.
func (h *KubernetesHandle) waitForPod(ctx context.Context) (string, error) {
	// Poll for pod creation
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
			pods, err := h.clientset.CoreV1().Pods(h.namespace).List(ctx, metav1.ListOptions{
				LabelSelector: fmt.Sprintf("job-name=%s", h.jobName),
			})
			if err != nil {
				return "", err
			}
			if len(pods.Items) > 0 {
				return pods.Items[0].Name, nil
			}
		}
	}
}

// Stop deletes the Kubernetes Job.
func (h *KubernetesHandle) Stop(ctx context.Context) error {
	// Delete with foreground propagation to clean up pods
	propagation := metav1.DeletePropagationForeground
	err := h.clientset.BatchV1().Jobs(h.namespace).Delete(ctx, h.jobName, metav1.DeleteOptions{
		PropagationPolicy: &propagation,
	})
	if err != nil {
		return fmt.Errorf("failed to delete job %s: %w", h.jobName, err)
	}
	log.Printf("Deleted Kubernetes Job %s", h.jobName)
	return nil
}

// StreamLogs returns a reader for the job's pod logs.
func (h *KubernetesHandle) StreamLogs(ctx context.Context) (io.ReadCloser, error) {
	// Wait for pod name if not yet known
	if h.podName == "" {
		podName, err := h.waitForPod(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to find pod for job %s: %w", h.jobName, err)
		}
		h.podName = podName
	}

	// Wait for container to be running or completed
	if err := h.waitForContainerReady(ctx); err != nil {
		return nil, err
	}

	req := h.clientset.CoreV1().Pods(h.namespace).GetLogs(h.podName, &corev1.PodLogOptions{
		Container: "job",
		Follow:    true,
	})

	return req.Stream(ctx)
}

// ResultDir returns the directory where the result of this job is stored.
func (h *KubernetesHandle) ResultDir() string {
	return "" // Results not yet supported on Kubernetes runtime
}

// Cleanup removes the temporary directory where the result of this job is stored.
// This is called by the worker after the job is completed.
func (h *KubernetesHandle) Cleanup() error {
	return nil
}

// waitForContainerReady waits for the container to start (or complete).
func (h *KubernetesHandle) waitForContainerReady(ctx context.Context) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			pod, err := h.clientset.CoreV1().Pods(h.namespace).Get(ctx, h.podName, metav1.GetOptions{})
			if err != nil {
				return err
			}
			// Ready if running, succeeded, or failed
			switch pod.Status.Phase {
			case corev1.PodRunning, corev1.PodSucceeded, corev1.PodFailed:
				return nil
			}
		}
	}
}
