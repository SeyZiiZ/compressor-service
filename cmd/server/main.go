package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"github.com/video-compressor/internal/api"
	"github.com/video-compressor/internal/broker"
	"github.com/video-compressor/internal/config"
	"github.com/video-compressor/internal/storage"
	"github.com/video-compressor/internal/worker"

	_ "github.com/video-compressor/docs"
)

// @title           Video Compressor API
// @version         1.0
// @description     Microservice for video and image compression. Submit jobs, track progress via SSE, and download compressed files.
// @host            localhost:8080
// @BasePath        /
// @schemes         http
func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// Database
	db, err := sql.Open("postgres", cfg.Database.DSN())
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		log.Fatalf("failed to ping database: %v", err)
	}
	log.Println("connected to database")

	// Run migrations
	if err := runMigrations(db); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	// Redis
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("failed to connect to redis: %v", err)
	}
	defer rdb.Close()
	log.Println("connected to redis")

	// Local staging dir for inputs + temp outputs (always needed: ffmpeg/vips work on files).
	if err := os.MkdirAll(cfg.Storage.BasePath, 0755); err != nil {
		log.Fatalf("failed to create storage dir: %v", err)
	}

	// Output storage backend: S3 (push compressed results to a bucket) or local disk.
	var store storage.Storage
	switch cfg.Storage.Driver {
	case "s3":
		store, err = storage.NewS3Storage(storage.S3Options{
			Endpoint:        cfg.Storage.S3.Endpoint,
			Region:          cfg.Storage.S3.Region,
			Bucket:          cfg.Storage.S3.Bucket,
			AccessKeyID:     cfg.Storage.S3.AccessKeyID,
			SecretAccessKey: cfg.Storage.S3.SecretAccessKey,
			ForcePathStyle:  cfg.Storage.S3.ForcePathStyle,
		})
	default:
		store, err = storage.NewLocalStorage(cfg.Storage.BasePath)
	}
	if err != nil {
		log.Fatalf("failed to init storage backend (%s): %v", cfg.Storage.Driver, err)
	}
	log.Printf("storage backend: %s", cfg.Storage.Driver)

	// Webhook client for completion callbacks to the calling backend.
	webhookClient := worker.NewWebhookClient(cfg.Webhook.Secret, cfg.Webhook.TimeoutSeconds)

	// RabbitMQ
	rmq, err := broker.NewRabbitMQ(cfg.RabbitMQ.URL)
	if err != nil {
		log.Fatalf("failed to connect to RabbitMQ: %v", err)
	}
	defer rmq.Close()
	log.Println("connected to RabbitMQ")

	// Context with graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Worker pool
	w := worker.New(db, rdb, store, webhookClient)
	pool := worker.NewPool(w, rmq)
	go func() {
		if err := pool.Start(ctx, cfg.Worker.VideoWorkers, cfg.Worker.ImageWorkers); err != nil {
			log.Printf("worker pool error: %v", err)
		}
	}()
	log.Printf("worker pool started (video=%d, image=%d)", cfg.Worker.VideoWorkers, cfg.Worker.ImageWorkers)

	// HTTP API
	handler := api.NewHandler(db, rdb, rmq, store, cfg.Storage.BasePath)
	router := api.SetupRouter(handler, cfg.Server.APIKey)
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: router,
	}

	go func() {
		log.Printf("API server starting on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down...")

	// Graceful shutdown
	cancel() // stop workers

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown error: %v", err)
	}

	log.Println("server stopped")
}

func runMigrations(db *sql.DB) error {
	migration := `
	CREATE TABLE IF NOT EXISTS jobs (
		id              VARCHAR(36) PRIMARY KEY,
		status          VARCHAR(20) NOT NULL DEFAULT 'pending',
		type            VARCHAR(10) NOT NULL,
		original_filename VARCHAR(255) NOT NULL DEFAULT '',
		input_path      TEXT NOT NULL DEFAULT '',
		output_path     TEXT NOT NULL DEFAULT '',
		progress        INTEGER NOT NULL DEFAULT 0,
		input_size      BIGINT NOT NULL DEFAULT 0,
		output_size     BIGINT NOT NULL DEFAULT 0,
		compression_ratio DOUBLE PRECISION NOT NULL DEFAULT 0,
		options_json    JSONB NOT NULL DEFAULT '{}',
		error           TEXT,
		webhook_url     TEXT NOT NULL DEFAULT '',
		created_at      TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
		updated_at      TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
		completed_at    TIMESTAMP WITH TIME ZONE
	);
	CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
	CREATE INDEX IF NOT EXISTS idx_jobs_created_at ON jobs(created_at DESC);`

	_, err := db.Exec(migration)
	return err
}
