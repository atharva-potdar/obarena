package main

import (
	"context"
	"log/slog"

	metricsv1 "iicpc-sh26/gen/proto/obarena/v1"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sr"
)

type Consumer struct {
	client *kgo.Client
	serde  *sr.Serde
	topic  string
}

func NewConsumer(brokers []string, topic string, serde *sr.Serde) (*Consumer, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumerGroup("telemetry-ingester"),
		kgo.ConsumeTopics(topic),
		kgo.DisableAutoCommit(),
	)
	if err != nil {
		return nil, err
	}
	return &Consumer{client: client, serde: serde, topic: topic}, nil
}

func (c *Consumer) Run(ctx context.Context, handler func(context.Context, *metricsv1.BotMetrics)) {
	for {
		fetches := c.client.PollFetches(ctx)
		if ctx.Err() != nil {
			return
		}
		fetches.EachError(func(t string, p int32, err error) {
			slog.Error("fetch error", "topic", t, "partition", p, "err", err)
		})
		fetches.EachRecord(func(r *kgo.Record) {
			// Validate magic byte before attempting decode
			if len(r.Value) < 5 || r.Value[0] != 0x00 {
				slog.Error("malformed payload: missing confluent header",
					"topic", r.Topic,
					"partition", r.Partition,
					"offset", r.Offset,
				)
				return
			}

			decoded, err := c.serde.DecodeNew(r.Value)
			if err != nil {
				slog.Error("decode bot.metrics", "err", err,
					"topic", r.Topic,
					"partition", r.Partition,
					"offset", r.Offset,
				)
				return
			}

			event, ok := decoded.(*metricsv1.BotMetrics)
			if !ok {
				slog.Error("unexpected type from decode", "type_got", decoded)
				return
			}

			handler(ctx, event)
		})

		if err := c.client.CommitUncommittedOffsets(ctx); err != nil {
			slog.Error("commit offsets", "err", err)
		}
	}
}

func (c *Consumer) Close() {
	c.client.Close()
}
