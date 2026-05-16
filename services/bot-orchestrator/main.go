package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

type Server struct {
	ctx          context.Context
	orchestrator *Orchestrator
	mu           sync.Mutex
	isRunning    bool
}

func (s *Server) runHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.Lock()
	if s.isRunning {
		s.mu.Unlock()
		http.Error(w, "Test already in progress", http.StatusConflict)
		return
	}
	s.isRunning = true
	s.mu.Unlock()

	var event SandboxReadyEvent
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&event)
	}

	// Default fallback if no body provided
	if event.SubmissionID == "" {
		event.SubmissionID = "manual-run"
	}
	if event.PodIP == "" {
		event.PodIP = "submission-api.platform.svc.cluster.local"
	}
	if event.WSPort == 0 {
		event.WSPort = 8080
	}
	if event.TeamName == "" {
		event.TeamName = "manual-team"
	}

	go func() {
		defer func() {
			s.mu.Lock()
			s.isRunning = false
			s.mu.Unlock()
		}()
		s.orchestrator.Handle(s.ctx, event)
	}()

	w.WriteHeader(http.StatusAccepted)
	if _, err := w.Write([]byte(`{"status": "started"}`)); err != nil {
		log.Printf("runHandler write error: %v", err)
	}
}

func (s *Server) statusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.Lock()
	running := s.isRunning
	s.mu.Unlock()

	status := "idle"
	if running {
		status = "running"
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": status}); err != nil {
		log.Printf("statusHandler encode error: %v", err)
	}
}

func main() {
	brokers := strings.Split(envStr("REDPANDA_BROKERS", "redpanda.platform.svc.cluster.local:9092"), ",")
	numBots := envInt("NUM_BOTS", 50)
	durationSec := envInt("DURATION_SECONDS", 60)
	jobTimeoutSec := envInt("JOB_TIMEOUT_SECONDS", 120)
	botRunnerImage := envStr("BOT_RUNNER_IMAGE", "bot-runner:dev")

	publisher, err := NewPublisher(brokers)
	if err != nil {
		log.Fatalf("init publisher: %v", err)
	}
	defer publisher.Close()

	cfg := Config{
		NumBots:         numBots,
		DurationSeconds: durationSec,
		JobTimeoutSec:   jobTimeoutSec,
		RedpandaBrokers: strings.Join(brokers, ","),
		BotRunnerImage:  botRunnerImage,
	}

	orchestrator, err := NewOrchestrator(publisher, cfg)
	if err != nil {
		log.Fatalf("init orchestrator: %v", err)
	}

	consumer, err := NewConsumer(brokers)
	if err != nil {
		log.Fatalf("init consumer: %v", err)
	}
	defer consumer.Close()

	log.Printf("bot-orchestrator started | numBots=%d | duration=%ds | jobTimeout=%ds",
		numBots, durationSec, jobTimeoutSec)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	srv := &Server{ctx: ctx, orchestrator: orchestrator}
	http.HandleFunc("/run", srv.runHandler)
	http.HandleFunc("/status", srv.statusHandler)
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("ok")); err != nil {
			log.Printf("healthz write error: %v", err)
		}
	})

	httpServer := &http.Server{Addr: ":8080"}

	// Run HTTP server
	go func() {
		log.Println("HTTP server listening on :8080")
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server: %v", err)
		}
	}()

	// Use consumer as well, forwarding to the same orchestrator (guarded by state if needed)
	go func() {
		consumer.Run(ctx, func(c context.Context, e SandboxReadyEvent) {
			srv.mu.Lock()
			if srv.isRunning {
				srv.mu.Unlock()
				log.Printf("ignoring sandbox.ready for %s because test is already running", e.SubmissionID)
				return
			}
			srv.isRunning = true
			srv.mu.Unlock()

			defer func() {
				srv.mu.Lock()
				srv.isRunning = false
				srv.mu.Unlock()
			}()

			orchestrator.Handle(c, e)
		})
	}()

	<-ctx.Done()
	log.Println("shutting down")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("http server shutdown error: %v", err)
	}
}
