// Package main is the entry point for the jobplane controller.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"jobplane/internal/config"
	"jobplane/internal/controller"
	"jobplane/internal/store/postgres"
)

func main() {
	// Load Config
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Setup Database
	ctx := context.Background()
	// Connect to Postgres (the "Store")
	store, err := postgres.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to DB: %v", err)
	}
	defer store.Close()

	// Start Server
	addr := fmt.Sprintf(":%d", cfg.HTTPPort)
	srv := controller.New(addr, store)

	go func() {
		log.Printf("JobPlane Controller starting on %s", addr)
		if err := srv.Run(ctx); err != nil {
			log.Printf("Server stopped: %v", err)
		}
	}()

	// Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down controller...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	log.Println("Server exited properly")
}
