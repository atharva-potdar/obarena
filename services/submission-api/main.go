package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt64(key string, def int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}

func run() error {
	seaweedfsEndpoint := envStr("SEAWEEDFS_ENDPOINT", "http://seaweedfs.platform.svc.cluster.local:8333")
	redpandaBrokers := strings.Split(envStr("REDPANDA_BROKERS", "redpanda.platform.svc.cluster.local:9092"), ",")
	port := envStr("PORT", "8080")
	maxUploadMB := envInt64("MAX_UPLOAD_SIZE_MB", 128)

	storage, err := NewStorage(seaweedfsEndpoint)
	if err != nil {
		return fmt.Errorf("init storage: %v", err)
	}

	publisher, err := NewPublisher(redpandaBrokers)
	if err != nil {
		return fmt.Errorf("init publisher: %v", err)
	}
	defer publisher.Close()

	h := NewHandler(storage, publisher, maxUploadMB)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"status":"ok"}`)); err != nil {
			log.Printf("healthz write error: %v", err)
		}
	})
	mux.HandleFunc("POST /submissions", h.handleSubmit)

	log.Printf("submission-api listening :%s", port)

	srv := &http.Server{Addr: ":" + port, Handler: mux}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown error: %v", err)
		}
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}

	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
