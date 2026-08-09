package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cloudtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sfn"
	"github.com/ogtrading/overnight-strategy/internal/packagevalidator"
)

type service struct {
	s3                                                                      *s3.Client
	registry                                                                *dynamodb.Client
	workflow                                                                *sfn.Client
	metrics                                                                 *cloudwatch.Client
	bucket, table, stateMachine, landingPrefix, rawPrefix, quarantinePrefix string
}

func main() {
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		panic(err)
	}
	s := &service{s3: s3.NewFromConfig(cfg), registry: dynamodb.NewFromConfig(cfg), workflow: sfn.NewFromConfig(cfg), metrics: cloudwatch.NewFromConfig(cfg), bucket: os.Getenv("DATA_LAKE_BUCKET"), table: os.Getenv("PACKAGE_REGISTRY_TABLE"), stateMachine: os.Getenv("STATE_MACHINE_ARN"), landingPrefix: os.Getenv("LANDING_PREFIX"), rawPrefix: os.Getenv("RAW_PREFIX"), quarantinePrefix: os.Getenv("QUARANTINE_PREFIX")}
	lambda.Start(s.handle)
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
			if !strings.HasPrefix(key, s.landingPrefix) || !strings.HasSuffix(key, "/MANIFEST.json") {
				continue
			}
			if err := s.validate(ctx, record.S3.Bucket.Name, key, record.S3.Object.VersionID, record.EventTime); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *service) validate(ctx context.Context, bucket, key, version string, eventTime time.Time) error {
	manifestBody, err := s.get(ctx, bucket, key, version)
	if err != nil {
		return err
	}
	defer manifestBody.Close()
	var manifest packagevalidator.Manifest
	if err := json.NewDecoder(io.LimitReader(manifestBody, 4<<20)).Decode(&manifest); err != nil {
		return err
	}
	if manifest.PackageID == "" {
		return fmt.Errorf("manifest package_id is required")
	}
	created := eventTime.UTC().Format(time.RFC3339Nano)
	if eventTime.IsZero() {
		created = time.Now().UTC().Format(time.RFC3339Nano)
	}
	_, err = s.registry.PutItem(ctx, &dynamodb.PutItemInput{TableName: aws.String(s.table), ConditionExpression: aws.String("attribute_not_exists(package_id)"), Item: map[string]ddbtypes.AttributeValue{"package_id": &ddbtypes.AttributeValueMemberS{Value: manifest.PackageID}, "session_date": &ddbtypes.AttributeValueMemberS{Value: manifest.Date}, "manifest_version": &ddbtypes.AttributeValueMemberN{Value: fmt.Sprint(manifest.SchemaVersion)}, "s3_bucket": &ddbtypes.AttributeValueMemberS{Value: bucket}, "s3_key": &ddbtypes.AttributeValueMemberS{Value: key}, "s3_version": &ddbtypes.AttributeValueMemberS{Value: version}, "validation_status": &ddbtypes.AttributeValueMemberS{Value: "VALIDATING"}, "processing_status": &ddbtypes.AttributeValueMemberS{Value: "NOT_STARTED"}, "created_at": &ddbtypes.AttributeValueMemberS{Value: created}}})
	if err != nil {
		var duplicate *ddbtypes.ConditionalCheckFailedException
		if errors.As(err, &duplicate) {
			return nil
		}
		return err
	}

	prefix := strings.TrimSuffix(key, "MANIFEST.json")
	validationErrors := []string{}
	for _, file := range manifest.Files {
		objectKey := prefix + file.Path
		head, headErr := s.s3.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(objectKey)})
		if headErr != nil {
			validationErrors = append(validationErrors, "MISSING:"+file.Path)
			continue
		}
		if file.Compressed > 0 && aws.ToInt64(head.ContentLength) != file.Compressed {
			validationErrors = append(validationErrors, "SIZE:"+file.Path)
			continue
		}
		body, getErr := s.get(ctx, bucket, objectKey, "")
		if getErr != nil {
			validationErrors = append(validationErrors, "READ:"+file.Path)
			continue
		}
		h := sha256.New()
		_, copyErr := io.Copy(h, body)
		_ = body.Close()
		if copyErr != nil || !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), file.SHA256) {
			validationErrors = append(validationErrors, "CHECKSUM:"+file.Path)
		}
	}
	if !manifest.Complete {
		validationErrors = append(validationErrors, "MANIFEST_INCOMPLETE")
	}
	if len(validationErrors) > 0 {
		return s.reject(ctx, manifest.PackageID, validationErrors)
	}

	for _, file := range append(manifest.Files, packagevalidator.File{Path: "MANIFEST.json"}) {
		sourceKey := prefix + file.Path
		destination := strings.TrimSuffix(s.rawPrefix, "/") + "/" + strings.TrimPrefix(sourceKey, s.landingPrefix)
		_, err = s.s3.CopyObject(ctx, &s3.CopyObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(destination), CopySource: aws.String(url.PathEscape(bucket + "/" + sourceKey))})
		if err != nil {
			return err
		}
	}
	validated := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.registry.UpdateItem(ctx, &dynamodb.UpdateItemInput{TableName: aws.String(s.table), Key: map[string]ddbtypes.AttributeValue{"package_id": &ddbtypes.AttributeValueMemberS{Value: manifest.PackageID}}, UpdateExpression: aws.String("SET validation_status=:v, checksum_status=:c, validated_at=:t, processing_status=:p"), ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{":v": &ddbtypes.AttributeValueMemberS{Value: "VALID"}, ":c": &ddbtypes.AttributeValueMemberS{Value: "PASS"}, ":t": &ddbtypes.AttributeValueMemberS{Value: validated}, ":p": &ddbtypes.AttributeValueMemberS{Value: "QUEUED"}}})
	if err != nil {
		return err
	}
	ack, _ := json.Marshal(map[string]any{"package_id": manifest.PackageID, "status": "VALID", "validated_at": validated, "shadow_only": true})
	if _, err := s.s3.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(strings.TrimSuffix(s.landingPrefix, "/") + "/acknowledgements/" + manifest.PackageID + ".json"), Body: strings.NewReader(string(ack)), ContentType: aws.String("application/json")}); err != nil {
		return err
	}
	input, _ := json.Marshal(map[string]any{"package_id": manifest.PackageID, "session_date": manifest.Date, "bucket": s.bucket, "raw_prefix": strings.TrimSuffix(s.rawPrefix, "/") + "/" + strings.TrimPrefix(prefix, s.landingPrefix), "shadow_only": true})
	_, err = s.workflow.StartExecution(ctx, &sfn.StartExecutionInput{StateMachineArn: aws.String(s.stateMachine), Name: aws.String(executionName(manifest.PackageID)), Input: aws.String(string(input))})
	return err
}

func (s *service) reject(ctx context.Context, packageID string, reasons []string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	reason := strings.Join(reasons, ",")
	_, _ = s.registry.UpdateItem(ctx, &dynamodb.UpdateItemInput{TableName: aws.String(s.table), Key: map[string]ddbtypes.AttributeValue{"package_id": &ddbtypes.AttributeValueMemberS{Value: packageID}}, UpdateExpression: aws.String("SET validation_status=:v, checksum_status=:c, failure_reason=:r, validated_at=:t"), ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{":v": &ddbtypes.AttributeValueMemberS{Value: "QUARANTINED"}, ":c": &ddbtypes.AttributeValueMemberS{Value: "FAIL"}, ":r": &ddbtypes.AttributeValueMemberS{Value: reason}, ":t": &ddbtypes.AttributeValueMemberS{Value: now}}})
	marker, _ := json.Marshal(map[string]any{"package_id": packageID, "reasons": reasons, "quarantined_at": now})
	_, putErr := s.s3.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(strings.TrimSuffix(s.quarantinePrefix, "/") + "/" + packageID + ".json"), Body: strings.NewReader(string(marker)), ContentType: aws.String("application/json")})
	value := 1.0
	_, _ = s.metrics.PutMetricData(ctx, &cloudwatch.PutMetricDataInput{Namespace: aws.String("OvernightStrategy/DataQuality"), MetricData: []cloudtypes.MetricDatum{{MetricName: aws.String("PackageQuarantined"), Value: aws.Float64(value), Unit: cloudtypes.StandardUnitCount}}})
	if strings.Contains(reason, "CHECKSUM:") {
		_, _ = s.metrics.PutMetricData(ctx, &cloudwatch.PutMetricDataInput{Namespace: aws.String("OvernightStrategy/DataQuality"), MetricData: []cloudtypes.MetricDatum{{MetricName: aws.String("ChecksumMismatch"), Value: aws.Float64(value), Unit: cloudtypes.StandardUnitCount}}})
	}
	return putErr
}

func (s *service) get(ctx context.Context, bucket, key, version string) (io.ReadCloser, error) {
	input := &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)}
	if version != "" {
		input.VersionId = aws.String(version)
	}
	object, err := s.s3.GetObject(ctx, input)
	if err != nil {
		return nil, err
	}
	return object.Body, nil
}
func executionName(id string) string {
	clean := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_", r) {
			return r
		}
		return '-'
	}, path.Base(id))
	if len(clean) > 80 {
		return clean[:80]
	}
	return clean
}
