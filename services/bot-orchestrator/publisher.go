package main

import (
	"context"
	"fmt"
	"os"
	"time"

	lifecyclev1 "iicpc-sh26/gen/proto/obarena/v1"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sr"
)

type Publisher struct {
	client *kgo.Client
	serde  *sr.Serde
	topic  string
}

func NewPublisher(brokers []string, topic string, serde *sr.Serde) (*Publisher, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.WithLogger(kgo.BasicLogger(os.Stderr, kgo.LogLevelInfo, nil)),
	)
	if err != nil {
		return nil, fmt.Errorf("new kafka client: %w", err)
	}
	return &Publisher{client: client, serde: serde, topic: topic}, nil
}

func (p *Publisher) PublishTestComplete(ctx context.Context, e TestCompleteEvent) error {
	event := &lifecyclev1.LifecycleEvent{
		Event: &lifecyclev1.LifecycleEvent_TestComplete{
			TestComplete: &lifecyclev1.TestComplete{
				SubmissionId: e.SubmissionID,
				TeamName:     e.TeamName,
				Success:      e.Success,
				Reason:       e.Reason,
				CompletedAt:  time.Now().UnixNano(),
			},
		},
	}

	payload, err := p.serde.Encode(event)
	if err != nil {
		return fmt.Errorf("encode event: %w", err)
	}

	record := &kgo.Record{
		Topic: p.topic,
		Key:   []byte(e.SubmissionID),
		Value: payload,
	}

	pCtx, pCancel := context.WithTimeout(ctx, 10*time.Second)
	defer pCancel()
	if err := p.client.ProduceSync(pCtx, record).FirstErr(); err != nil {
		return fmt.Errorf("produce: %w", err)
	}
	return nil
}

func (p *Publisher) Close() {
	p.client.Close()
}

// TestCompleteEvent is the local struct used by the orchestrator to pass data
// to the publisher. It mirrors the protobuf TestComplete message fields.
type TestCompleteEvent struct {
	SubmissionID string
	TeamName     string
	Success      bool
	Reason       string
}
