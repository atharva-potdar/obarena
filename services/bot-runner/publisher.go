package main

import (
	"context"
	"fmt"
	"time"

	metricsv1 "iicpc-sh26/gen/proto/obarena/v1"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sr"
)

func publishMetrics(client *kgo.Client, serde *sr.Serde, topic string, agg *AggregateMetrics, duration time.Duration, teamName, submissionID, testRunID string, correctnessScore float64) error {
	if client == nil {
		return nil
	}

	tps := float64(agg.ordersSent) / duration.Seconds()

	event := &metricsv1.BotMetrics{
		TeamName:         teamName,
		SubmissionId:     submissionID,
		TestRunId:        testRunID,
		DurationMs:       duration.Milliseconds(),
		OrdersSent:       agg.ordersSent,
		AcksRecv:         agg.acksRecv,
		FillsRecv:        agg.fillsRecv,
		RejectsRecv:      agg.rejectsRecv,
		StaleOrders:      agg.staleOrders,
		AckP50Us:         agg.ackLatency.ValueAtQuantile(50),
		AckP90Us:         agg.ackLatency.ValueAtQuantile(90),
		AckP99Us:         agg.ackLatency.ValueAtQuantile(99),
		AckP999Us:        agg.ackLatency.ValueAtQuantile(99.9),
		AckMaxUs:         agg.ackLatency.Max(),
		FillP50Us:        agg.fillLatency.ValueAtQuantile(50),
		FillP90Us:        agg.fillLatency.ValueAtQuantile(90),
		FillP99Us:        agg.fillLatency.ValueAtQuantile(99),
		FillP999Us:       agg.fillLatency.ValueAtQuantile(99.9),
		FillMaxUs:        agg.fillLatency.Max(),
		Tps:              tps,
		CorrectnessScore: correctnessScore,
		EmittedAt:        time.Now().UnixNano(),
	}

	payload, err := serde.Encode(event)
	if err != nil {
		return fmt.Errorf("encode event: %w", err)
	}

	record := &kgo.Record{
		Topic: topic,
		Key:   []byte(submissionID),
		Value: payload,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.ProduceSync(ctx, record).FirstErr(); err != nil {
		return fmt.Errorf("produce: %w", err)
	}
	return nil
}
