// Package main is the entry point for the jobplane worker.
// The worker is the "Muscle" agent that executes jobs.
// It owns concurrency, timeouts, retries, and process/runtime management.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"jobplane/internal/config"
	"jobplane/internal/observability"
	"jobplane/internal/worker"
	"jobplane/internal/worker/runtime"
)

func main() {
	// Parse flags
	configPath := flag.String("config", "", "Path to config file (default: jobplane.yaml in current directory)")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Tracing
	shutdownTracer, err := observability.InitTracer(ctx, "jobplane-worker", cfg.OTELEndpoint)
	if err != nil {
		log.Fatalf("Failed to init tracing: %v", err)
	}
	defer func() {
		if err := shutdownTracer(context.Background()); err != nil {
			log.Printf("Failed to shutdown tracer: %v", err)
		}
	}()

	// Select runtime based on configuration
	var rt runtime.Runtime
	switch cfg.Runtime {
	case "exec":
		rt = runtime.NewExecRuntime(cfg.RuntimeWorkDir)
		log.Printf("Using exec runtime (workdir: %s)", cfg.RuntimeWorkDir)
	case "kubernetes":
		k8sRT, err := runtime.NewKubernetesRuntime(runtime.KubernetesConfig{
			Namespace:          cfg.KubernetesNamespace,
			ServiceAccount:     cfg.KubernetesServiceAccount,
			DefaultCPULimit:    cfg.KubernetesCPULimit,
			DefaultMemoryLimit: cfg.KubernetesMemoryLimit,
		})
		if err != nil {
			log.Fatalf("Failed to create Kubernetes runtime: %v", err)
		}
		rt = k8sRT
		log.Printf("Using kubernetes runtime (namespace: %s)", cfg.KubernetesNamespace)
	case "docker":
		fallthrough
	default:
		dockerRT, err := runtime.NewDockerRuntime()
		if err != nil {
			log.Fatalf("Failed to create Docker runtime: %v", err)
		}
		rt = dockerRT
		log.Println("Using docker runtime")
	}

	agent := worker.New(rt, worker.AgentConfig{
		Concurrency:       cfg.WorkerConcurrency,
		PollInterval:      cfg.WorkerPollInterval,
		ControllerURL:     cfg.ControllerURL,
		MaxBackoff:        cfg.WorkerMaxBackoff,
		HeartbeatInterval: cfg.WorkerHeartbeatInterval,
	}, nil)

	log.Printf("Worker started with concurrency %d", cfg.WorkerConcurrency)
	go agent.Run(ctx)

	// Metrics
	metricsHandler, shutdownMetrics, err := observability.InitMetrics()
	if err != nil {
		log.Fatalf("Failed to init metrics: %v", err)
	}
	defer func() {
		if err := shutdownMetrics(context.Background()); err != nil {
			log.Printf("Failed to shutdown metrics: %v", err)
		}
	}()

	// Start a dedicated metrics server on port 6162
	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", metricsHandler)
		log.Println("Worker metrics listening on :6162")
		if err := http.ListenAndServe(":6162", mux); err != nil {
			log.Printf("Metrics server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down worker...")
	cancel()

	<-agent.Done()
}
