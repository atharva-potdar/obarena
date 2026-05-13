package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type EventScorePair struct {
	Event BotMetricsEvent
	Score float64
}

type Ingester struct {
	db                 *pgxpool.Pool
	redis              *redis.Client
	maxAcceptableP90US float64
	maxAcceptableTPS   float64

	mu          sync.Mutex
	eventBuffer []EventScorePair
	flushTicker *time.Ticker
	closeCh     chan struct{}
}

func NewIngester(dsn, redisAddr string, maxP90US, maxTPS float64) (*Ingester, error) {
	db, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		return nil, fmt.Errorf("create pgxpool: %w", err)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})

	ingester := &Ingester{
		db:                 db,
		redis:              rdb,
		maxAcceptableP90US: maxP90US,
		maxAcceptableTPS:   maxTPS,
		eventBuffer:        make([]EventScorePair, 0, 1000),
		flushTicker:        time.NewTicker(500 * time.Millisecond),
		closeCh:            make(chan struct{}),
	}

	go ingester.flushLoop()

	return ingester, nil
}

func (i *Ingester) flushLoop() {
	for {
		select {
		case <-i.flushTicker.C:
			i.flush(context.Background())
		case <-i.closeCh:
			i.flushTicker.Stop()
			i.flush(context.Background()) // Final flush
			return
		}
	}
}

func (i *Ingester) flush(ctx context.Context) {
	i.mu.Lock()
	if len(i.eventBuffer) == 0 {
		i.mu.Unlock()
		return
	}
	bufferToFlush := make([]EventScorePair, len(i.eventBuffer))
	copy(bufferToFlush, i.eventBuffer)
	i.eventBuffer = i.eventBuffer[:0]
	i.mu.Unlock()

	if err := i.writeBatch(ctx, bufferToFlush); err != nil {
		log.Printf("flush batch error: %v", err)
	}
}

func (i *Ingester) writeBatch(ctx context.Context, pairs []EventScorePair) error {
	batch := &pgx.Batch{}

	for _, p := range pairs {
		batch.Queue(`
			INSERT INTO telemetry_events
				(time, submission_id, bot_id, event_type, latency_us, order_id)
			VALUES
				($1, $2, $3, $4, $5, NULL)
		`, time.Unix(0, p.Event.EmittedAt), p.Event.SubmissionID, p.Event.TestRunID, "bot.metrics", p.Event.AckP90US)

		batch.Queue(`
			INSERT INTO submission_scores
				(submission_id, team_name, p50_us, p90_us, p99_us, tps, correctness, composite, scored_at)
			VALUES
				($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (submission_id) DO UPDATE SET
				p50_us      = EXCLUDED.p50_us,
				p90_us      = EXCLUDED.p90_us,
				p99_us      = EXCLUDED.p99_us,
				tps         = EXCLUDED.tps,
				correctness = EXCLUDED.correctness,
				composite   = EXCLUDED.composite,
				scored_at   = EXCLUDED.scored_at
		`, p.Event.SubmissionID, p.Event.TeamName, p.Event.AckP50US, p.Event.AckP90US, p.Event.AckP99US, p.Event.TPS,
			1.0-(float64(p.Event.RejectsRecv)/math.Max(1, float64(p.Event.OrdersSent))), p.Score, time.Now())
	}

	br := i.db.SendBatch(ctx, batch)
	defer br.Close()

	for j := 0; j < len(pairs)*2; j++ {
		_, err := br.Exec()
		if err != nil {
			return fmt.Errorf("batch exec error at index %d: %w", j, err)
		}
	}

	return nil
}

func (i *Ingester) Handle(ctx context.Context, event BotMetricsEvent) {
	score := i.computeScore(event)

	i.mu.Lock()
	i.eventBuffer = append(i.eventBuffer, EventScorePair{Event: event, Score: score})
	i.mu.Unlock()

	if err := i.updateLeaderboard(ctx, event, score); err != nil {
		log.Printf("update leaderboard: %v", err)
	}

	log.Printf("ingested: submission=%s score=%.4f tps=%.0f ack_p90=%dµs",
		event.SubmissionID, score, event.TPS, event.AckP90US)
}

func (i *Ingester) writeTelemetry(ctx context.Context, event BotMetricsEvent) error {
	_, err := i.db.Exec(
		ctx, `
		INSERT INTO telemetry_events
			(time, submission_id, bot_id, event_type, latency_us, order_id)
		VALUES
			($1, $2, $3, $4, $5, NULL)
	`,
		time.Unix(0, event.EmittedAt),
		event.SubmissionID,
		event.TestRunID,
		"bot.metrics",
		event.AckP90US,
	)
	return err
}

func (i *Ingester) computeScore(event BotMetricsEvent) float64 {
	latencyScore := 1.0 - (float64(event.AckP90US) / i.maxAcceptableP90US)
	latencyScore = math.Max(0, math.Min(1, latencyScore))

	throughputScore := event.TPS / i.maxAcceptableTPS
	throughputScore = math.Max(0, math.Min(1, throughputScore))

	correctnessScore := 1.0
	if event.OrdersSent > 0 {
		correctnessScore = 1.0 - (float64(event.RejectsRecv) / float64(event.OrdersSent))
		correctnessScore = math.Max(0, math.Min(1, correctnessScore))
	}

	return (latencyScore * 0.4) + (throughputScore * 0.4) + (correctnessScore * 0.2)
}

func (i *Ingester) updateLeaderboard(ctx context.Context, event BotMetricsEvent, score float64) error {
	member := fmt.Sprintf("%s:%s", event.SubmissionID, event.TeamName)
	err := i.redis.ZAdd(ctx, "leaderboard", redis.Z{
		Score:  score * 1000,
		Member: member,
	}).Err()
	if err != nil {
		return err
	}

	payload := fmt.Sprintf(`{"submission_id": "%s", "team_name": "%s", "score": %.4f}`, event.SubmissionID, event.TeamName, score)
	return i.redis.Publish(ctx, "leaderboard_updates", payload).Err()
}

func (i *Ingester) Close() {
	close(i.closeCh)
	time.Sleep(100 * time.Millisecond) // wait for flushLoop to complete final flush
	i.db.Close()
	i.redis.Close()
}
