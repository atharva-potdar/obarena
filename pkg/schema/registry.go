// Package schema provides a reusable Schema Registry client and Serde factory
// for Confluent wire format (Magic Byte 0x00 + 4-byte Schema ID + protobuf index
// + protobuf bytes). It wraps github.com/twmb/franz-go/pkg/sr to handle schema
// registration, compatibility configuration, and zero-reflection serialization.
package schema

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/twmb/franz-go/pkg/sr"
	"google.golang.org/protobuf/proto"
)

// SubjectConfig describes a schema subject to register on startup.
type SubjectConfig struct {
	// Subject is the Schema Registry subject name (e.g. "submission.lifecycle-value").
	Subject string
	// ProtoSchema is the raw .proto file content to register.
	ProtoSchema string
	// Compatibility is the desired compatibility level for this subject.
	Compatibility sr.CompatibilityLevel
}

// RegisteredSchema is the result of registering a schema subject.
type RegisteredSchema struct {
	Subject string
	ID      int
}

// Registry wraps an sr.Client for schema registration and compatibility management.
type Registry struct {
	client *sr.Client
}

// NewRegistry creates a Schema Registry client connected to the given URL.
func NewRegistry(url string) (*Registry, error) {
	client, err := sr.NewClient(sr.URLs(url))
	if err != nil {
		return nil, fmt.Errorf("schema registry client: %w", err)
	}
	return &Registry{client: client}, nil
}

// Register idempotently registers a protobuf schema under the given subject,
// sets the compatibility level, and returns the globally unique schema ID.
func (r *Registry) Register(ctx context.Context, cfg SubjectConfig) (RegisteredSchema, error) {
	s := sr.Schema{
		Schema: cfg.ProtoSchema,
		Type:   sr.TypeProtobuf,
	}

	ss, err := r.client.CreateSchema(ctx, cfg.Subject, s)
	if err != nil {
		return RegisteredSchema{}, fmt.Errorf("register schema %q: %w", cfg.Subject, err)
	}

	results := r.client.SetCompatibility(ctx, sr.SetCompatibility{
		Level: cfg.Compatibility,
	}, cfg.Subject)
	for _, result := range results {
		if result.Err != nil {
			slog.Warn("set compatibility failed",
				"subject", cfg.Subject,
				"level", cfg.Compatibility,
				"err", result.Err,
			)
		}
	}

	slog.Info("schema registered",
		"subject", cfg.Subject,
		"id", ss.ID,
		"version", ss.Version,
		"compatibility", cfg.Compatibility,
	)

	return RegisteredSchema{Subject: cfg.Subject, ID: ss.ID}, nil
}

// Binding associates a registered schema ID with a Go protobuf type for Serde registration.
type Binding struct {
	ID    int
	Type  proto.Message
	Index []int
}

// NewSerde creates an sr.Serde configured with ConfluentHeader (the Confluent/Redpanda
// wire format) and registers all provided bindings with proto.Marshal / proto.Unmarshal.
// No reflection is used in the hot-path encode/decode functions.
func NewSerde(bindings []Binding) *sr.Serde {
	serde := sr.NewSerde(
		sr.EncodeFn(func(v any) ([]byte, error) {
			return proto.Marshal(v.(proto.Message))
		}),
		sr.DecodeFn(func(b []byte, v any) error {
			return proto.Unmarshal(b, v.(proto.Message))
		}),
	)

	for _, b := range bindings {
		opts := []sr.EncodingOpt{
			sr.GenerateFn(generatorFor(b.Type)),
		}
		if len(b.Index) > 0 {
			opts = append(opts, sr.Index(b.Index...))
		} else {
			opts = append(opts, sr.Index(0))
		}
		serde.Register(b.ID, b.Type, opts...)
	}

	return serde
}

// generatorFor returns a function that allocates a new instance of the same
// concrete type as msg. This is used by Serde.DecodeNew to avoid reflection.
func generatorFor(msg proto.Message) func() any {
	return func() any {
		return proto.Clone(msg)
	}
}
