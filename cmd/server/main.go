package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/XferOps/hizal/internal/api"
	"github.com/XferOps/hizal/internal/db"
	"github.com/XferOps/hizal/internal/embeddings"
	"github.com/getsentry/sentry-go"
	sentryhttp "github.com/getsentry/sentry-go/http"
)

func main() {
	// Sentry: init early so panics during startup are captured too.
	// Gracefully no-ops if SENTRY_DSN is not set.
	if dsn := os.Getenv("SENTRY_DSN"); dsn != "" {
		if err := sentry.Init(sentry.ClientOptions{
			Dsn:              dsn,
			Release:          os.Getenv("VERSION"),
			Environment:      os.Getenv("APP_ENV"),
			TracesSampleRate: 0.1,
		}); err != nil {
			log.Printf("Warning: Sentry init failed: %v", err)
		} else {
			defer sentry.Flush(2 * time.Second)
			log.Printf("Sentry initialized (release=%s env=%s)", os.Getenv("VERSION"), os.Getenv("APP_ENV"))
		}
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	pool, err := db.Connect(context.Background())
	if err != nil {
		log.Printf("Warning: database connection failed: %v", err)
		pool = nil
	}
	if pool != nil {
		defer pool.Close()
		if err := db.RunMigrations(context.Background(), pool); err != nil {
			log.Fatalf("Failed to run migrations: %v", err)
		}
	}

	embed, err := embeddings.NewClient()
	if err != nil {
		log.Printf("Warning: embeddings client init failed: %v", err)
		embed = nil
	}

	router := api.NewRouter(pool, embed)

	// Wrap with Sentry middleware: captures panics, reports to Sentry,
	// and re-panics so the server's normal recovery path still fires.
	// sentryhttp.New is a no-op when Sentry is not initialized.
	sentryHandler := sentryhttp.New(sentryhttp.Options{
		Repanic: true,
	})

	srv := &http.Server{
		Addr:        fmt.Sprintf(":%s", port),
		Handler:     sentryHandler.Handle(router),
		ReadTimeout: 15 * time.Second,
		// SSE endpoints can legitimately stay open while incremental progress
		// events are streamed, so a fixed write timeout will cut them off.
		WriteTimeout: 0,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("Hizal server starting on port %s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	log.Println("Server stopped")
}
