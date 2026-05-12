package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

type TestCompleteEvent struct {
	Event        string `json:"event"`
	SubmissionID string `json:"submission_id"`
	TeamName     string `json:"team_name"`
	Success      bool   `json:"success"`
	Reason       string `json:"reason"`
	CompletedAt  int64  `json:"completed_at"`
}

type Publisher struct {
	client *kgo.Client
}

func NewPublisher(brokers []string) (*Publisher, error) {
	client, err := kgo.NewClient(kgo.SeedBrokers(brokers...))
	if err != nil {
		return nil, fmt.Errorf("new kafka client: %w", err)
	}
	return &Publisher{client: client}, nil
}

func (p *Publisher) PublishTestComplete(ctx context.Context, e TestCompleteEvent) error {
	e.Event = "test.complete"
	e.CompletedAt = time.Now().UnixNano()

	payload, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	record := &kgo.Record{
		Topic: "submission.lifecycle",
		Key:   []byte(e.SubmissionID),
		Value: payload,
	}

	if err := p.client.ProduceSync(ctx, record).FirstErr(); err != nil {
		return fmt.Errorf("produce: %w", err)
	}
	return nil
}

func (p *Publisher) Close() {
	p.client.Close()
}
