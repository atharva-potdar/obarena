package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
)

type Ingester struct {
	db                 *pgx.Conn
	redis              *redis.Client
	maxAcceptableP90US float64
	maxAcceptableTPS   float64
}

func NewIngester(dsn, redisAddr string, maxP90US, maxTPS float64) (*Ingester, error) {
	db, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		return nil, fmt.Errorf("connect timescaledb: %w", err)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})

	return &Ingester{
		db:                 db,
		redis:              rdb,
		maxAcceptableP90US: maxP90US,
		maxAcceptableTPS:   maxTPS,
	}, nil
}

func (i *Ingester) Handle(ctx context.Context, event BotMetricsEvent) {
	if err := i.writeTelemetry(ctx, event); err != nil {
		log.Printf("write telemetry: %v", err)
	}

	score := i.computeScore(event)

	if err := i.writeScore(ctx, event, score); err != nil {
		log.Printf("write score: %v", err)
	}

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
			($1, $2, $3, $4, $5, $6)
	`,
		time.Unix(0, event.EmittedAt),
		event.SubmissionID,
		event.TestRunID,
		"bot.metrics",
		event.AckP90US,
		event.SubmissionID,
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

func (i *Ingester) writeScore(ctx context.Context, event BotMetricsEvent, score float64) error {
	_, err := i.db.Exec(
		ctx, `
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
	`,
		event.SubmissionID,
		event.TeamName,
		event.AckP50US,
		event.AckP90US,
		event.AckP99US,
		event.TPS,
		1.0-(float64(event.RejectsRecv)/math.Max(1, float64(event.OrdersSent))),
		score,
		time.Now(),
	)
	return err
}

func (i *Ingester) updateLeaderboard(ctx context.Context, event BotMetricsEvent, score float64) error {
	member := fmt.Sprintf("%s:%s", event.SubmissionID, event.TeamName)
	return i.redis.ZAdd(ctx, "leaderboard", redis.Z{
		Score:  score * 1000,
		Member: member,
	}).Err()
}

func (i *Ingester) Close() {
	i.db.Close(context.Background())
	i.redis.Close()
}
