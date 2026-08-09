package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/ogtrading/overnight-strategy/internal/packagevalidator"
)

type service struct {
	s3         *s3.Client
	validation string
	quarantine string
}

func main() {
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		panic(err)
	}
	lambda.Start((&service{s3: s3.NewFromConfig(cfg), validation: os.Getenv("VALIDATION_BUCKET"), quarantine: os.Getenv("QUARANTINE_BUCKET")}).handle)
}

func (s *service) handle(ctx context.Context, queue events.SQSEvent) error {
	for _, message := range queue.Records {
		var event events.S3Event
		if err := json.Unmarshal([]byte(message.Body), &event); err != nil {
			return fmt.Errorf("decode S3 event: %w", err)
		}
		for _, record := range event.Records {
			key, err := url.QueryUnescape(record.S3.Object.Key)
			if err != nil {
				return err
			}
			if !strings.HasSuffix(key, "/MANIFEST.json") {
				continue
			}
			if err := s.validate(ctx, record.S3.Bucket.Name, key, record.S3.Object.Sequencer); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *service) validate(ctx context.Context, bucket, key, generation string) error {
	prefix := strings.TrimSuffix(key, "MANIFEST.json")
	temp, err := os.MkdirTemp("", "aws-package-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	manifestPath := filepath.Join(temp, "MANIFEST.json")
	if err := s.download(ctx, bucket, key, manifestPath); err != nil {
		return err
	}
	body, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}
	var manifest packagevalidator.Manifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return err
	}
	for _, file := range manifest.Files {
		if err := s.download(ctx, bucket, prefix+file.Path, filepath.Join(temp, filepath.FromSlash(file.Path))); err != nil {
			return err
		}
	}
	result := packagevalidator.Validate(temp)
	output, _ := json.MarshalIndent(map[string]any{"validated_at": time.Now().UTC(), "source_bucket": bucket, "manifest_object": key, "s3_sequencer": generation, "result": result}, "", "  ")
	output = append(output, '\n')
	target := s.validation
	targetPrefix := "validation"
	if !result.Valid {
		target = s.quarantine
		targetPrefix = "quarantine"
	}
	if target == "" {
		return fmt.Errorf("validation target bucket is not configured")
	}
	_, err = s.s3.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(target), Key: aws.String(targetPrefix + "/" + manifest.PackageID + ".json"), Body: strings.NewReader(string(output)), ContentType: aws.String("application/json")})
	return err
}

func (s *service) download(ctx context.Context, bucket, key, path string) error {
	object, err := s.s3.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		return err
	}
	defer object.Body.Close()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, object.Body)
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
