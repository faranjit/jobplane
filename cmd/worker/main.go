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

	docker, err := runtime.NewDockerRuntime()
	if err != nil {
		log.Fatalf("Failed to create Docker runtime: %v", err)
	}

	agent := worker.New(store, docker, worker.AgentConfig{
		Concurrency:  cfg.WorkerConcurrency,
		PollInterval: cfg.WorkerPollInterval,
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
