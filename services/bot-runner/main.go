package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	endpoint := envStr("TARGET_ENDPOINT", "ws://localhost:8080/stream")
	numBots := envInt("NUM_BOTS", 10)
	durationSec := envInt("DURATION_SECONDS", 30)
	teamName := envStr("TEAM_NAME", "unknown")
	submissionID := envStr("TEST_RUN_ID", "")
	testRunID := submissionID
	brokers := strings.Split(envStr("REDPANDA_BROKERS", ""), ",")

	duration := time.Duration(durationSec) * time.Second

	var client *kgo.Client
	if len(brokers) > 0 && brokers[0] != "" {
		var err error
		client, err = kgo.NewClient(kgo.SeedBrokers(brokers...))
		if err != nil {
			log.Fatalf("failed to create kafka client: %v", err)
		}
		defer client.Close()
	}

	// ── Phase 1: Correctness (synchronous, 1 bot, ~5-10s) ──────────────
	log.Printf("phase 1: correctness validation against %s", endpoint)
	cb := NewCorrectnessBot(endpoint)
	correctnessResult := cb.Run(context.Background())
	log.Printf("correctness: score=%.4f (%d/%d passed)",
		correctnessResult.Score,
		correctnessResult.Passed,
		correctnessResult.TotalAssertions)

	// ── Phase 2: Load test (async, N bots, duration) ────────────────────
	log.Printf("phase 2: load test (%d bots, %s) against %s",
		numBots, duration, endpoint)

	ctx, cancel := context.WithTimeout(context.Background(), duration+30*time.Second)
	defer cancel()

	bots := make([]*Bot, numBots)
	for i := range bots {
		seqs := sequences(i)
		seq := seqs[i%len(seqs)]
		bots[i] = NewBot(i, endpoint, seq)
	}

	readyCh := make(chan struct{}, numBots)
	var wg sync.WaitGroup
	for _, bot := range bots {
		wg.Add(1)
		go func(b *Bot) {
			defer wg.Done()
			b.Run(ctx, duration, readyCh)
		}(bot)
	}

	log.Printf("waiting for all bots to warm up...")
	for i := 0; i < numBots; i++ {
		select {
		case <-readyCh:
		case <-ctx.Done():
			return
		}
	}
	log.Printf("all bots warmed up, starting measurement")
	start := time.Now()

	wg.Wait()
	elapsed := time.Since(start)

	metrics := make([]*BotMetrics, numBots)
	for i, b := range bots {
		metrics[i] = b.metrics
	}
	agg := merge(metrics)
	report(agg, elapsed)

	log.Printf("attempting to publish metrics: submission=%s brokers=%v", submissionID, brokers)
	if submissionID != "" {
		if err := publishMetrics(client, agg, elapsed, teamName, submissionID, testRunID, correctnessResult.Score); err != nil {
			log.Printf("failed to publish metrics: %v", err)
		} else {
			log.Printf("metrics published to Redpanda")
		}
	}

	if client != nil {
		flushCtx, flushCancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := client.Flush(flushCtx); err != nil {
			log.Printf("kafka flush error: %v", err)
		}
		flushCancel()
	}
	log.Printf("Closing bot runner")
}
