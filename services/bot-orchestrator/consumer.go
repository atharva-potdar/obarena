package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	lifecyclev1 "iicpc-sh26/gen/proto/obarena/v1"

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
		kgo.ConsumerGroup("bot-orchestrator"),
		kgo.ConsumeTopics(topic),
		kgo.DisableAutoCommit(),
		kgo.WithLogger(kgo.BasicLogger(os.Stderr, kgo.LogLevelInfo, nil)),
	)
	if err != nil {
		return nil, err
	}
	return &Consumer{client: client, serde: serde, topic: topic}, nil
}

// SandboxReadyEvent is used by the HTTP handler and Kafka consumer handler.
// It wraps the protobuf SandboxReady data for the orchestrator.Handle interface.
type SandboxReadyEvent struct {
	SubmissionID string
	PodName      string
	PodIP        string
	HTTPPort     int
	WSPort       int
	TeamName     string
}

func (c *Consumer) Run(ctx context.Context, handler func(context.Context, SandboxReadyEvent)) {
	for {
		fetches := c.client.PollFetches(ctx)
		if ctx.Err() != nil {
			return
		}
		fetches.EachError(func(t string, p int32, err error) {
			slog.Error("fetch error", "topic", t, "partition", p, "err", err)
		})
		fetches.EachRecord(func(r *kgo.Record) {
			decoded, err := c.serde.DecodeNew(r.Value)
			if err != nil {
				slog.Warn("decode error in record", "error", err, "topic", r.Topic, "partition", r.Partition, "offset", r.Offset)
				return
			}

			envelope, ok := decoded.(*lifecyclev1.LifecycleEvent)
			if !ok {
				slog.Warn("unexpected type from decode", "type", fmt.Sprintf("%T", decoded))
				return
			}

			sr := envelope.GetSandboxReady()
			if sr == nil {
				return // not a sandbox.ready event
			}

			event := SandboxReadyEvent{
				SubmissionID: sr.SubmissionId,
				PodName:      sr.PodName,
				PodIP:        sr.PodIp,
				HTTPPort:     int(sr.HttpPort),
				WSPort:       int(sr.WsPort),
				TeamName:     sr.TeamName,
			}

			go func() {
				defer func() {
					if r := recover(); r != nil {
						slog.Error("panic in consumer handler", "error", r)
					}
				}()
				handler(ctx, event)
				if err := c.client.CommitRecords(ctx, r); err != nil {
					slog.Error("failed to commit record", "error", err)
				}
			}()
		})
	}
}

func (c *Consumer) Close() {
	c.client.Close()
}
