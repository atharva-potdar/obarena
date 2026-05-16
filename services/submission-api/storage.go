package main

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const defaultBucket = "submissions"

type Storage struct {
	client *s3.Client
	bucket string
}

func NewStorage(endpoint string, bucket string) (*Storage, error) {
	if bucket == "" {
		bucket = defaultBucket
	}
	cfg, err := config.LoadDefaultConfig(
		context.Background(),
		config.WithRegion(envStr("AWS_REGION", "us-east-1")),
	)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})

	return &Storage{client: client, bucket: bucket}, nil
}

func (s *Storage) Upload(ctx context.Context, key string, r io.Reader) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   r,
	})
	if err != nil {
		return fmt.Errorf("put object %s: %w", key, err)
	}
	return nil
}

func (s *Storage) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("delete object %s: %w", key, err)
	}
	return nil
}
