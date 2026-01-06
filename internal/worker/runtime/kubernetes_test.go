package runtime

import (
	"context"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestKubernetesRuntime_Start_CreatesJob(t *testing.T) {
	clientset := fake.NewClientset()

	rt := &KubernetesRuntime{
		clientset: clientset,
		config: KubernetesConfig{
			Namespace:          "test-ns",
			DefaultCPULimit:    "500m",
			DefaultMemoryLimit: "256Mi",
		},
	}

	ctx := context.Background()
	handle, err := rt.Start(ctx, StartOptions{
		Image:   "alpine:latest",
		Command: []string{"echo", "hello"},
		Env:     map[string]string{"FOO": "bar"},
	})

	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	if handle == nil {
		t.Fatal("expected handle to be non-nil")
	}

	// Verify job was created
	jobs, err := clientset.BatchV1().Jobs("test-ns").List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("failed to list jobs: %v", err)
	}

	if len(jobs.Items) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs.Items))
	}

	job := jobs.Items[0]

	// Verify job spec
	if job.Spec.Template.Spec.Containers[0].Image != "alpine:latest" {
		t.Errorf("expected image alpine:latest, got %s", job.Spec.Template.Spec.Containers[0].Image)
	}

	if len(job.Spec.Template.Spec.Containers[0].Command) != 2 {
		t.Errorf("expected 2 command args, got %d", len(job.Spec.Template.Spec.Containers[0].Command))
	}

	// Verify labels
	if job.Labels["app.kubernetes.io/managed-by"] != "jobplane" {
		t.Error("expected managed-by label to be 'jobplane'")
	}
}

func TestKubernetesRuntime_Start_WithServiceAccount(t *testing.T) {
	clientset := fake.NewClientset()

	rt := &KubernetesRuntime{
		clientset: clientset,
		config: KubernetesConfig{
			Namespace:          "test-ns",
			ServiceAccount:     "my-sa",
			DefaultCPULimit:    "500m",
			DefaultMemoryLimit: "256Mi",
		},
	}

	ctx := context.Background()
	_, err := rt.Start(ctx, StartOptions{
		Image:   "alpine:latest",
		Command: []string{"echo"},
	})

	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	jobs, _ := clientset.BatchV1().Jobs("test-ns").List(ctx, metav1.ListOptions{})
	job := jobs.Items[0]

	if job.Spec.Template.Spec.ServiceAccountName != "my-sa" {
		t.Errorf("expected service account 'my-sa', got '%s'", job.Spec.Template.Spec.ServiceAccountName)
	}
}

func TestKubernetesHandle_Stop_DeletesJob(t *testing.T) {
	existingJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "test-ns",
		},
	}
	clientset := fake.NewClientset(existingJob)

	handle := &KubernetesHandle{
		clientset: clientset,
		namespace: "test-ns",
		jobName:   "test-job",
	}

	ctx := context.Background()
	err := handle.Stop(ctx)

	if err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}

	// Verify job was deleted
	jobs, _ := clientset.BatchV1().Jobs("test-ns").List(ctx, metav1.ListOptions{})
	if len(jobs.Items) != 0 {
		t.Errorf("expected 0 jobs after delete, got %d", len(jobs.Items))
	}
}

func TestKubernetesRuntime_Start_SetsResourceLimits(t *testing.T) {
	clientset := fake.NewClientset()

	rt := &KubernetesRuntime{
		clientset: clientset,
		config: KubernetesConfig{
			Namespace:          "test-ns",
			DefaultCPULimit:    "1",
			DefaultMemoryLimit: "512Mi",
		},
	}

	ctx := context.Background()
	_, err := rt.Start(ctx, StartOptions{
		Image:   "alpine:latest",
		Command: []string{"echo"},
	})

	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	jobs, _ := clientset.BatchV1().Jobs("test-ns").List(ctx, metav1.ListOptions{})
	container := jobs.Items[0].Spec.Template.Spec.Containers[0]

	cpuLimit := container.Resources.Limits.Cpu().String()
	memLimit := container.Resources.Limits.Memory().String()

	if cpuLimit != "1" {
		t.Errorf("expected CPU limit '1', got '%s'", cpuLimit)
	}

	if memLimit != "512Mi" {
		t.Errorf("expected memory limit '512Mi', got '%s'", memLimit)
	}
}

func TestKubernetesRuntime_Start_SetsBackoffLimitToZero(t *testing.T) {
	clientset := fake.NewClientset()

	rt := &KubernetesRuntime{
		clientset: clientset,
		config: KubernetesConfig{
			Namespace:          "test-ns",
			DefaultCPULimit:    "500m",
			DefaultMemoryLimit: "256Mi",
		},
	}

	ctx := context.Background()
	_, _ = rt.Start(ctx, StartOptions{
		Image:   "alpine:latest",
		Command: []string{"echo"},
	})

	jobs, _ := clientset.BatchV1().Jobs("test-ns").List(ctx, metav1.ListOptions{})
	job := jobs.Items[0]

	// BackoffLimit should be 0 since jobplane handles retries
	if *job.Spec.BackoffLimit != 0 {
		t.Errorf("expected backoff limit 0, got %d", *job.Spec.BackoffLimit)
	}
}

func TestKubernetesHandle_WaitForPod_FindsPod(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-ns",
			Labels:    map[string]string{"job-name": "test-job"},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}
	clientset := fake.NewClientset(pod)

	handle := &KubernetesHandle{
		clientset: clientset,
		namespace: "test-ns",
		jobName:   "test-job",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	podName, err := handle.waitForPod(ctx)
	if err != nil {
		t.Fatalf("waitForPod failed: %v", err)
	}

	if podName != "test-pod" {
		t.Errorf("expected pod name 'test-pod', got '%s'", podName)
	}
}

func TestKubernetesHandle_WaitForPod_Timeout(t *testing.T) {
	// Empty clientset - no pods will be found
	clientset := fake.NewClientset()

	handle := &KubernetesHandle{
		clientset: clientset,
		namespace: "test-ns",
		jobName:   "test-job",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := handle.waitForPod(ctx)

	if err == nil {
		t.Error("expected timeout error, got nil")
	}
}

func TestKubernetesHandle_WaitForContainerReady(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-ns",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}
	clientset := fake.NewClientset(pod)

	handle := &KubernetesHandle{
		clientset: clientset,
		namespace: "test-ns",
		jobName:   "test-job",
		podName:   "test-pod",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := handle.waitForContainerReady(ctx)
	if err != nil {
		t.Fatalf("waitForContainerReady failed: %v", err)
	}
}

func TestKubernetesRuntime_Start_SetsEnvVars(t *testing.T) {
	clientset := fake.NewClientset()

	rt := &KubernetesRuntime{
		clientset: clientset,
		config: KubernetesConfig{
			Namespace:          "test-ns",
			DefaultCPULimit:    "500m",
			DefaultMemoryLimit: "256Mi",
		},
	}

	ctx := context.Background()
	_, err := rt.Start(ctx, StartOptions{
		Image:   "alpine:latest",
		Command: []string{"echo"},
		Env: map[string]string{
			"FOO": "bar",
			"BAZ": "qux",
		},
	})

	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	jobs, _ := clientset.BatchV1().Jobs("test-ns").List(ctx, metav1.ListOptions{})
	container := jobs.Items[0].Spec.Template.Spec.Containers[0]

	if len(container.Env) != 2 {
		t.Errorf("expected 2 env vars, got %d", len(container.Env))
	}

	// Check that env vars are present (order may vary)
	envMap := make(map[string]string)
	for _, env := range container.Env {
		envMap[env.Name] = env.Value
	}

	if envMap["FOO"] != "bar" {
		t.Errorf("expected FOO=bar, got FOO=%s", envMap["FOO"])
	}
	if envMap["BAZ"] != "qux" {
		t.Errorf("expected BAZ=qux, got BAZ=%s", envMap["BAZ"])
	}
}
