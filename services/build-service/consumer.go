package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	lifecyclev1 "iicpc-sh26/gen/proto/obarena/v1"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sr"
)

type Consumer struct {
	client    *kgo.Client
	builder   *Builder
	publisher *Publisher
	serde     *sr.Serde
	topic     string
}

func NewConsumer(brokers []string, topic string, builder *Builder, publisher *Publisher, serde *sr.Serde) (*Consumer, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumerGroup("build-service"),
		kgo.ConsumeTopics(topic),
		kgo.DisableAutoCommit(),
	)
	if err != nil {
		return nil, fmt.Errorf("new kafka client: %w", err)
	}
	return &Consumer{client: client, builder: builder, publisher: publisher, serde: serde, topic: topic}, nil
}

// Run consumes events until the context is cancelled.
func (c *Consumer) Run(ctx context.Context) error {
	for {
		fetches := c.client.PollFetches(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errs := fetches.Errors(); len(errs) > 0 {
			for _, e := range errs {
				slog.Error("fetch error", "topic", e.Topic, "partition", e.Partition, "error", e.Err)
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(1 * time.Second):
			}
			continue
		}

		for _, record := range fetches.Records() {
			if err := c.handleRecord(ctx, record); err != nil {
				return fmt.Errorf("handle record: %w", err)
			}
			if err := c.client.CommitRecords(ctx, record); err != nil {
				return fmt.Errorf("commit record: %w", err)
			}
		}
	}
}

func (c *Consumer) handleRecord(ctx context.Context, record *kgo.Record) error {
	decoded, err := c.serde.DecodeNew(record.Value)
	if err != nil {
		slog.Error("decode event", "error", err, "offset", record.Offset)
		return nil // skip malformed events
	}

	envelope, ok := decoded.(*lifecyclev1.LifecycleEvent)
	if !ok {
		slog.Error("unexpected type from decode", "type", fmt.Sprintf("%T", decoded))
		return nil
	}

	sc := envelope.GetSubmissionCreated()
	if sc == nil {
		return nil // not a submission.created event
	}

	slog.Info("processing build",
		"submission", sc.SubmissionId,
		"lang", sc.Language,
		"team", sc.TeamName,
	)

	result, err := c.builder.Build(ctx, sc)
	if err != nil {
		slog.Error("build failed", "submission", sc.SubmissionId, "error", err)
		if pubErr := c.publisher.PublishBuildFailed(ctx, sc.SubmissionId, err.Error()); pubErr != nil {
			slog.Error("publish build.failed", "error", pubErr)
		}
		return nil // we published a failure, so this record is "handled"
	}

	slog.Info("build complete", "submission", sc.SubmissionId, "binary", result.BinaryPath)
	if pubErr := c.publisher.PublishBuildComplete(
		ctx, sc.SubmissionId, result.BinaryPath, sc.Language, sc.TeamName,
	); pubErr != nil {
		return fmt.Errorf("publish build.complete: %w", pubErr)
	}

	return nil
}

func (c *Consumer) Close() {
	c.client.Close()
}
