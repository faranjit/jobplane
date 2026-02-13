// Package main is the entry point for the jobplane controller.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"jobplane/internal/config"
	"jobplane/internal/controller"
	"jobplane/internal/observability"
	"jobplane/internal/store/postgres"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

func main() {
	// Parse flags
	migrateFlag := flag.Bool("migrate", false, "Run database migrations before starting")
	configPath := flag.String("config", "", "Path to config file (default: jobplane.yaml in current directory)")
	flag.Parse()

	// Load Config
	cfg, err := config.Load(*configPath)
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

	// Run migrations if requested
	if *migrateFlag {
		log.Println("Running database migrations...")
		if err := postgres.Migrate(store.DB()); err != nil {
			log.Fatalf("Migration failed: %v", err)
		}
		log.Println("Migrations completed successfully")
	}

	// Tracing
	shutdownTracer, err := observability.InitTracer(ctx, "jobplane-controller", cfg.OTELEndpoint)
	if err != nil {
		log.Fatalf("Failed to init tracing: %v", err)
	}
	defer func() {
		if err := shutdownTracer(context.Background()); err != nil {
			log.Printf("Failed to shutdown tracer: %v", err)
		}
	}()

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

	// Use an Observable Gauge (Async) that queries the DB only when scraped.
	meter := otel.Meter("jobplane-controller")
	_, err = meter.Int64ObservableGauge("jobplane.queue.depth",
		metric.WithDescription("Current number of jobs in the queue"),
		metric.WithInt64Callback(func(ctx context.Context, obs metric.Int64Observer) error {
			count, err := store.Count(ctx)
			if err != nil {
				log.Printf("Failed to count queue depth: %v", err)
				return nil // Don't crash metrics scrape on DB error
			}
			obs.Observe(count)
			return nil
		}),
	)
	if err != nil {
		log.Printf("Failed to register queue depth metric: %v", err)
	}

	// Start Server
	addr := fmt.Sprintf(":%d", cfg.HTTPPort)
	srv := controller.New(addr, store, cfg, metricsHandler)

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
