# Telemetry Ingester

## Overview

Consumes `bot.metrics` events from Redpanda. For each event, writes raw
telemetry to TimescaleDB for historical analysis and computes a composite
score which is written to Redis for live leaderboard ranking.

Single responsibility — no HTTP endpoints, no k8s API calls, no bot logic.
Runs as a single consumer in the `platform` namespace.

---

## Event Consumption

Topic:          bot.metrics
Consumer group: telemetry-ingester
Events handled: bot.metrics

---

## Ingestion Flow

1. Consume `bot.metrics` event
2. Buffer events in memory. Every 500ms flush via pgx.Batch to TimescaleDB `telemetry_events` table
3. Compute composite score from event metrics
4. Buffer scores in memory. Every 500ms flush via pgx.Batch to TimescaleDB `submission_scores` table
5. Update Redis sorted set with composite score (ZADD leaderboard)
6. Publish JSON payload to Redis pub/sub channel `leaderboard_updates` for real-time WebSocket push

---

## Composite Score Formula

The composite score is a weighted combination of three dimensions:

  score = (latency_score *0.4) + (throughput_score* 0.4) + (correctness_score * 0.2)

Where:

  latency_score    = 1 - (ack_p99_us / max_acceptable_p99_us)
                     clamped to [0, 1]
                     max_acceptable_p99_us = 100,000 (100ms)

  throughput_score = tps / max_acceptable_tps
                     clamped to [0, 1]
                     max_acceptable_tps = 10,000

  correctness_score = 1 - (rejects_recv / orders_sent)
                      clamped to [0, 1]
                      0 rejects = perfect correctness score

Final score is in range [0, 1]. Higher is better.

---

## Storage

### TimescaleDB

Table: telemetry_events
One row per bot.metrics event received.

Table: submission_scores
One row per submission. Upserted on each event — last write wins.

### Redis

Key:   leaderboard
Type:  Sorted Set
Score: composite score * 1000 (integer, higher is better)
Member: "{submission_id}:{team_name}"

ZADD leaderboard <score> <member>
PUBLISH leaderboard_updates {"submission_id": "uuid", "team_name": "name", "score": 0.8500}

The leaderboard frontend reads from this sorted set via ZREVRANGE
to get rankings in descending score order on startup, and subscribes to `leaderboard_updates` for real-time live updates.

---

## Event Schema

Consumed from topic: bot.metrics

{
  "event":          "bot.metrics",
  "submission_id":  "uuid",
  "test_run_id":    "uuid",
  "duration_ms":    60000,
  "orders_sent":    178276,
  "acks_recv":      356552,
  "fills_recv":     180404,
  "rejects_recv":   0,
  "conn_drops":     6,
  "ack_p50_us":     1445,
  "ack_p90_us":     14319,
  "ack_p99_us":     55711,
  "ack_p999_us":    70655,
  "ack_max_us":     90879,
  "fill_p50_us":    1613,
  "fill_p90_us":    14791,
  "fill_p99_us":    60639,
  "fill_p999_us":   71359,
  "fill_max_us":    90879,
  "tps":            2970.72,
  "emitted_at":     1234567890
}

---

## Configuration

REDPANDA_BROKERS        comma-separated broker list
                        default: redpanda.platform.svc.cluster.local:9092
TIMESCALEDB_DSN         postgres connection string
                        default: postgres://postgres:iicpc@timescaledb.platform.svc.cluster.local:5432/iicpc
REDIS_ADDR              Redis address
                        default: redis.platform.svc.cluster.local:6379
MAX_ACCEPTABLE_P99_US   p99 latency ceiling for scoring
                        default: 100000
MAX_ACCEPTABLE_TPS      TPS ceiling for scoring
                        default: 10000

---

## Dependencies

- github.com/twmb/franz-go/pkg/kgo    Redpanda consumer
- github.com/jackc/pgx/v5             TimescaleDB client
- github.com/jackc/pgx/v5/pgxpool     TimescaleDB connection pool
- github.com/redis/go-redis/v9         Redis client

---

## Constraints

- Telemetry ingester is not on the critical path — a slow write to
  TimescaleDB does not block the pipeline
- If TimescaleDB write fails, log and continue — do not crash
- If Redis write fails, log and continue — do not crash
- Consumer group offset is committed after both writes succeed
- One ingester instance — no concurrency needed at this scale
