package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	lifecyclev1 "iicpc-sh26/gen/proto/obarena/v1"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sr"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
)

type Consumer struct {
	client       *kgo.Client
	serde        *sr.Serde
	orchestrator *Orchestrator
	publisher    *Publisher
	topic        string
}

func NewConsumer(brokers []string, topic string, orchestrator *Orchestrator, publisher *Publisher, serde *sr.Serde) (*Consumer, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumerGroup("sandbox-orchestrator"),
		kgo.ConsumeTopics(topic),
		kgo.DisableAutoCommit(),
	)
	if err != nil {
		return nil, fmt.Errorf("new kafka client: %w", err)
	}
	return &Consumer{client: client, serde: serde, orchestrator: orchestrator, publisher: publisher, topic: topic}, nil
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
			if err := func() (err error) {
				defer func() {
					if r := recover(); r != nil {
						err = fmt.Errorf("panic in handleRecord: %v", r)
						slog.Error("recovered from panic", "error", err)
					}
				}()
				return c.handleRecord(ctx, record)
			}(); err != nil {
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

	bc := envelope.GetBuildComplete()
	if bc == nil {
		return nil // not a build.complete event
	}

	slog.Info("processing sandbox",
		"submission", bc.SubmissionId,
		"team", bc.TeamName,
	)

	result, err := c.orchestrator.Deploy(ctx, bc)
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			slog.Info("sandbox already exists, treating as success", "submission", bc.SubmissionId)
			return nil
		}
		slog.Error("sandbox failed", "submission", bc.SubmissionId, "error", err)
		if pubErr := c.publisher.PublishSandboxFailed(ctx, bc.SubmissionId, err.Error()); pubErr != nil {
			slog.Error("publish sandbox.failed", "error", pubErr)
		}
		return nil // sandbox failure is a business error, not a consumer error
	}

	slog.Info("sandbox ready",
		"submission", bc.SubmissionId,
		"pod", result.PodName,
		"ip", result.PodIP,
	)
	if pubErr := c.publisher.PublishSandboxReady(
		ctx, bc.SubmissionId, result.PodName, result.PodIP, bc.TeamName,
	); pubErr != nil {
		return fmt.Errorf("publish sandbox.ready: %w", pubErr)
	}

	return nil
}

func (c *Consumer) Close() {
	c.client.Close()
}
