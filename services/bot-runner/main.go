package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"sync"
	"time"
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

	duration := time.Duration(durationSec) * time.Second

	log.Printf("starting %d bots | duration=%s | target=%s",
		numBots, duration, endpoint)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bots := make([]*Bot, numBots)
	for i := range bots {
		seqs := sequences(i)
		seq := seqs[i%len(seqs)]
		bots[i] = NewBot(i, endpoint, seq)
	}

	start := time.Now()
	var wg sync.WaitGroup
	for _, bot := range bots {
		wg.Add(1)
		go func(b *Bot) {
			defer wg.Done()
			b.Run(ctx, duration)
		}(bot)
	}
	wg.Wait()
	elapsed := time.Since(start)

	metrics := make([]*BotMetrics, numBots)
	for i, b := range bots {
		metrics[i] = b.metrics
	}
	report(merge(metrics), elapsed)
}
