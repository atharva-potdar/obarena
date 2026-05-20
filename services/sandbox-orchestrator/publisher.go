package main

import (
	"context"
	"fmt"
	"os"
	"time"
	"unicode/utf8"

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

func (p *Publisher) PublishSandboxReady(
	ctx context.Context,
	submissionID, podName, podIP, teamName string,
) error {
	event := &lifecyclev1.LifecycleEvent{
		Event: &lifecyclev1.LifecycleEvent_SandboxReady{
			SandboxReady: &lifecyclev1.SandboxReady{
				SubmissionId: submissionID,
				PodName:      podName,
				PodIp:        podIP,
				HttpPort:     httpPort,
				WsPort:       httpPort,
				TeamName:     teamName,
				ReadyAt:      time.Now().UnixNano(),
			},
		},
	}
	return p.publish(ctx, submissionID, event)
}

func (p *Publisher) PublishSandboxFailed(
	ctx context.Context,
	submissionID, reason string,
) error {
	event := &lifecyclev1.LifecycleEvent{
		Event: &lifecyclev1.LifecycleEvent_SandboxFailed{
			SandboxFailed: &lifecyclev1.SandboxFailed{
				SubmissionId: submissionID,
				Reason:       truncate(reason, 4096),
				FailedAt:     time.Now().UnixNano(),
			},
		},
	}
	return p.publish(ctx, submissionID, event)
}

func (p *Publisher) publish(ctx context.Context, key string, event *lifecyclev1.LifecycleEvent) error {
	payload, err := p.serde.Encode(event)
	if err != nil {
		return fmt.Errorf("encode event: %w", err)
	}
	record := &kgo.Record{
		Topic: p.topic,
		Key:   []byte(key),
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

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	s = s[:max]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}
