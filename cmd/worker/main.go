// Package main is the entry point for the jobplane worker.
// The worker is the "Muscle" agent that executes jobs.
// It owns concurrency, timeouts, retries, and process/runtime management.
package main

import (
	"context"
	"jobplane/internal/config"
	"jobplane/internal/store/postgres"
	"jobplane/internal/worker"
	"jobplane/internal/worker/runtime"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store, err := postgres.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to create database connection: %v", err)
	}
	defer store.Close()

	// Select runtime based on configuration
	var rt runtime.Runtime
	switch cfg.Runtime {
	case "exec":
		rt = runtime.NewExecRuntime(cfg.RuntimeWorkDir)
		log.Printf("Using exec runtime (workdir: %s)", cfg.RuntimeWorkDir)
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

	agent := worker.New(store, rt, worker.AgentConfig{
		Concurrency:         cfg.WorkerConcurrency,
		PollInterval:        cfg.WorkerPollInterval,
		ControllerURL:       cfg.ControllerURL,
		MaxBackoff:          cfg.WorkerMaxBackoff,
		HeartbeatInterval:   cfg.WorkerHeartbeatInterval,
		VisibilityExtension: cfg.WorkerVisibilityExtension,
	}, nil)

	log.Printf("Worker started with concurrency %d", cfg.WorkerConcurrency)
	go agent.Run(ctx)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down worker...")
	cancel()

	<-agent.Done()
}
