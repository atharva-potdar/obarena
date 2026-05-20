// Package main implements the build service, which consumes submission events,
// orchestrates compilation via isolated Kubernetes pods, and publishes build results.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	lifecyclev1 "iicpc-sh26/gen/proto/obarena/v1"
	"iicpc-sh26/pkg/schema"

	"github.com/twmb/franz-go/pkg/sr"
)

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func run() error {
	seaweedfsEndpoint := envStr("SEAWEEDFS_ENDPOINT", "http://seaweedfs.platform.svc.cluster.local:8333")
	rawBrokers := envStr("REDPANDA_BROKERS", "redpanda.platform.svc.cluster.local:9092")
	var redpandaBrokers []string
	for _, b := range strings.Split(rawBrokers, ",") {
		if trimmed := strings.TrimSpace(b); trimmed != "" {
			redpandaBrokers = append(redpandaBrokers, trimmed)
		}
	}
	topic := envStr("KAFKA_TOPIC", "submission.lifecycle")
	schemaRegistryURL := envStr("SCHEMA_REGISTRY_URL", "http://redpanda.platform.svc.cluster.local:8081")
	maxLogBytes := envInt("MAX_LOG_BYTES", 4096)

	reg, err := schema.NewRegistry(schemaRegistryURL)
	if err != nil {
		return fmt.Errorf("schema registry: %v", err)
	}

	registered, err := reg.Register(context.Background(), schema.SubjectConfig{
		Subject:       topic + "-value",
		ProtoSchema:   schema.LifecycleProto,
		Compatibility: sr.CompatBackward,
	})
	if err != nil {
		return fmt.Errorf("register schema: %v", err)
	}

	serde := schema.NewSerde([]schema.Binding{
		{ID: registered.ID, Type: &lifecyclev1.LifecycleEvent{}, Index: []int{0}},
	})

	publisher, err := NewPublisher(redpandaBrokers, topic, serde)
	if err != nil {
		return fmt.Errorf("init publisher: %v", err)
	}
	defer publisher.Close()

	builder, err := NewBuilder(seaweedfsEndpoint, maxLogBytes)
	if err != nil {
		return fmt.Errorf("init builder: %v", err)
	}

	consumer, err := NewConsumer(redpandaBrokers, topic, builder, publisher, serde)
	if err != nil {
		return fmt.Errorf("init consumer: %v", err)
	}
	defer consumer.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	slog.Info("build-service starting")
	if err := consumer.Run(ctx); err != nil {
		slog.Error("consumer error", "err", err)
		if ctx.Err() == nil {
			return fmt.Errorf("consumer: %v", err)
		}
	}
	slog.Info("build-service stopped")

	return nil
}

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}
