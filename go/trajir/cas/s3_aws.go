package cas

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// S3API is the subset of the AWS S3 client used by AWSObjectAPI.
// *s3.Client implements this in production; tests inject fakes.
type S3API interface {
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
}

// AWSObjectAPI adapts an AWS SDK v2 S3 client to ObjectAPI.
type AWSObjectAPI struct {
	Client S3API
	// Ctx is used for all calls; defaults to context.Background when nil.
	Ctx context.Context
}

func (a *AWSObjectAPI) ctx() context.Context {
	if a.Ctx != nil {
		return a.Ctx
	}
	return context.Background()
}

// PutObject implements ObjectAPI.
func (a *AWSObjectAPI) PutObject(bucket, key string, body []byte) error {
	if a.Client == nil {
		return fmt.Errorf("%w: S3 client is nil", ErrCAS)
	}
	_, err := a.Client.PutObject(a.ctx(), &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(body),
	})
	return err
}

// GetObject implements ObjectAPI.
func (a *AWSObjectAPI) GetObject(bucket, key string) ([]byte, error) {
	if a.Client == nil {
		return nil, fmt.Errorf("%w: S3 client is nil", ErrCAS)
	}
	out, err := a.Client.GetObject(a.ctx(), &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isS3NotFound(err) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, key)
		}
		return nil, err
	}
	defer out.Body.Close()
	data, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// HeadObject implements ObjectAPI.
func (a *AWSObjectAPI) HeadObject(bucket, key string) error {
	if a.Client == nil {
		return fmt.Errorf("%w: S3 client is nil", ErrCAS)
	}
	_, err := a.Client.HeadObject(a.ctx(), &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isS3NotFound(err) {
			return fmt.Errorf("%w: %s", ErrNotFound, key)
		}
		return err
	}
	return nil
}

func isS3NotFound(err error) bool {
	if err == nil {
		return false
	}
	var nsk *types.NoSuchKey
	if errors.As(err, &nsk) {
		return true
	}
	var nsf *types.NotFound
	if errors.As(err, &nsf) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code := apiErr.ErrorCode()
		return code == "NotFound" || code == "NoSuchKey" || code == "404"
	}
	return false
}

// S3FromEnvOptions controls NewS3StoreFromEnv.
type S3FromEnvOptions struct {
	// Bucket overrides TRAJIR_S3_BUCKET.
	Bucket string
	// KeyPrefix is optional object key prefix (no leading slash).
	KeyPrefix string
	// Region defaults to AWS_REGION / AWS_DEFAULT_REGION / us-east-1.
	Region string
	// EndpointURL overrides TRAJIR_S3_ENDPOINT_URL (MinIO, LocalStack).
	EndpointURL string
}

// NewS3StoreFromEnv builds an S3Store using AWS SDK v2 and environment variables.
//
// Recognized variables (same spirit as Python build_s3_client_from_env):
//
//	AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY
//	AWS_REGION or AWS_DEFAULT_REGION (default us-east-1)
//	TRAJIR_S3_ENDPOINT_URL optional custom endpoint
//	TRAJIR_S3_BUCKET required unless opts.Bucket is set
//
// Path style addressing is enabled when an endpoint URL is set (MinIO).
func NewS3StoreFromEnv(ctx context.Context, opts S3FromEnvOptions) (*S3Store, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	bucket := strings.TrimSpace(opts.Bucket)
	if bucket == "" {
		bucket = strings.TrimSpace(os.Getenv("TRAJIR_S3_BUCKET"))
	}
	if bucket == "" {
		return nil, fmt.Errorf("%w: TRAJIR_S3_BUCKET or opts.Bucket is required", ErrCAS)
	}

	region := strings.TrimSpace(opts.Region)
	if region == "" {
		region = os.Getenv("AWS_REGION")
	}
	if region == "" {
		region = os.Getenv("AWS_DEFAULT_REGION")
	}
	if region == "" {
		region = "us-east-1"
	}

	endpoint := strings.TrimSpace(opts.EndpointURL)
	if endpoint == "" {
		endpoint = strings.TrimSpace(os.Getenv("TRAJIR_S3_ENDPOINT_URL"))
	}

	loadOpts := []func(*config.LoadOptions) error{
		config.WithRegion(region),
	}
	// Explicit static credentials when present (MinIO and CI often set these).
	ak := os.Getenv("AWS_ACCESS_KEY_ID")
	sk := os.Getenv("AWS_SECRET_ACCESS_KEY")
	if ak != "" && sk != "" {
		loadOpts = append(loadOpts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(ak, sk, os.Getenv("AWS_SESSION_TOKEN")),
		))
	}

	cfg, err := config.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("%w: load aws config: %v", ErrCAS, err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = true
		}
	})

	api := &AWSObjectAPI{Client: client, Ctx: ctx}
	return NewS3Store(api, bucket, opts.KeyPrefix)
}

// Ensure *s3.Client satisfies S3API at compile time.
var _ S3API = (*s3.Client)(nil)
