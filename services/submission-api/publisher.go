package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

type SubmissionCreatedEvent struct {
	Event        string `json:"event"`
	SubmissionID string `json:"submission_id"`
	Language     string `json:"language"`
	TeamName     string `json:"team_name"`
	ArtifactPath string `json:"artifact_path"`
	CreatedAt    int64  `json:"created_at"`
}

type Publisher struct {
	client *kgo.Client
}

func NewPublisher(brokers []string) (*Publisher, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.WithLogger(kgo.BasicLogger(os.Stderr, kgo.LogLevelInfo, nil)),
	)
	if err != nil {
		return nil, fmt.Errorf("new kafka client: %w", err)
	}
	return &Publisher{client: client}, nil
}

func (p *Publisher) PublishSubmissionCreated(ctx context.Context, e SubmissionCreatedEvent) error {
	e.Event = "submission.created"
	e.CreatedAt = time.Now().UnixNano()

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
