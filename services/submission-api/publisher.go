package main

import (
	"context"
	"fmt"
	"os"
	"time"

	lifecyclev1 "iicpc-sh26/gen/proto/obarena/v1"
	"iicpc-sh26/pkg/schema"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sr"
)

type Publisher struct {
	client *kgo.Client
	serde  *sr.Serde
	topic  string
}

func NewPublisher(brokers []string, topic, schemaRegistryURL string) (*Publisher, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.WithLogger(kgo.BasicLogger(os.Stderr, kgo.LogLevelInfo, nil)),
	)
	if err != nil {
		return nil, fmt.Errorf("new kafka client: %w", err)
	}

	reg, err := schema.NewRegistry(schemaRegistryURL)
	if err != nil {
		return nil, fmt.Errorf("schema registry: %w", err)
	}

	registered, err := reg.Register(context.Background(), schema.SubjectConfig{
		Subject:       topic + "-value",
		ProtoSchema:   schema.LifecycleProto,
		Compatibility: sr.CompatBackward,
	})
	if err != nil {
		return nil, fmt.Errorf("register schema: %w", err)
	}

	serde := schema.NewSerde([]schema.Binding{
		{ID: registered.ID, Type: &lifecyclev1.LifecycleEvent{}, Index: []int{0}},
	})

	return &Publisher{client: client, serde: serde, topic: topic}, nil
}

func (p *Publisher) PublishSubmissionCreated(ctx context.Context, submissionID, language, teamName, artifactPath string) error {
	event := &lifecyclev1.LifecycleEvent{
		Event: &lifecyclev1.LifecycleEvent_SubmissionCreated{
			SubmissionCreated: &lifecyclev1.SubmissionCreated{
				SubmissionId: submissionID,
				Language:     language,
				TeamName:     teamName,
				ArtifactPath: artifactPath,
				CreatedAt:    time.Now().UnixNano(),
			},
		},
	}

	payload, err := p.serde.Encode(event)
	if err != nil {
		return fmt.Errorf("encode event: %w", err)
	}

	record := &kgo.Record{
		Topic: p.topic,
		Key:   []byte(submissionID),
		Value: payload,
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := p.client.ProduceSync(ctx, record).FirstErr(); err != nil {
		return fmt.Errorf("produce: %w", err)
	}
	return nil
}

func (p *Publisher) Close() {
	p.client.Close()
}

