package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"strings"
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

	ctx := context.Background()
	consumer.Run(ctx, orchestrator.Handle)
}
