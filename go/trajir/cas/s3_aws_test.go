package cas

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

type fakeS3API struct {
	objects map[string][]byte
	putErr  error
	getErr  error
	headErr error
}

func newFakeS3API() *fakeS3API {
	return &fakeS3API{objects: map[string][]byte{}}
}

func (f *fakeS3API) key(bucket, key *string) string {
	b, k := "", ""
	if bucket != nil {
		b = *bucket
	}
	if key != nil {
		k = *key
	}
	return b + "/" + k
}

func (f *fakeS3API) PutObject(_ context.Context, in *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	if f.putErr != nil {
		return nil, f.putErr
	}
	data, err := io.ReadAll(in.Body)
	if err != nil {
		return nil, err
	}
	f.objects[f.key(in.Bucket, in.Key)] = data
	return &s3.PutObjectOutput{}, nil
}

func (f *fakeS3API) GetObject(_ context.Context, in *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	data, ok := f.objects[f.key(in.Bucket, in.Key)]
	if !ok {
		return nil, &types.NoSuchKey{Message: awsString("missing")}
	}
	return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(data))}, nil
}

func (f *fakeS3API) HeadObject(_ context.Context, in *s3.HeadObjectInput, _ ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	if f.headErr != nil {
		return nil, f.headErr
	}
	if _, ok := f.objects[f.key(in.Bucket, in.Key)]; !ok {
		return nil, &types.NotFound{Message: awsString("missing")}
	}
	return &s3.HeadObjectOutput{}, nil
}

func awsString(s string) *string { return &s }

type apiError string

func (e apiError) Error() string                { return string(e) }
func (e apiError) ErrorCode() string            { return string(e) }
func (e apiError) ErrorMessage() string         { return string(e) }
func (e apiError) ErrorFault() smithy.ErrorFault { return smithy.FaultUnknown }

func TestAWSObjectAPIRoundTrip(t *testing.T) {
	fake := newFakeS3API()
	api := &AWSObjectAPI{Client: fake}
	store, err := NewS3Store(api, "trajir", "")
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("from-aws-adapter")
	h, err := store.Put(payload)
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(h)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("mismatch")
	}
	if !store.Has(h) {
		t.Fatal("Has=false")
	}
}

func TestAWSObjectAPINotFound(t *testing.T) {
	fake := newFakeS3API()
	api := &AWSObjectAPI{Client: fake}
	_, err := api.GetObject("b", "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err=%v", err)
	}
	err = api.HeadObject("b", "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("head err=%v", err)
	}
}

func TestIsS3NotFoundAPIError(t *testing.T) {
	if !isS3NotFound(apiError("NoSuchKey")) {
		t.Fatal("want NoSuchKey")
	}
	if isS3NotFound(errors.New("AccessDenied")) {
		t.Fatal("AccessDenied is not missing")
	}
}

func TestNewS3StoreFromEnvRequiresBucket(t *testing.T) {
	t.Setenv("TRAJIR_S3_BUCKET", "")
	t.Setenv("AWS_ACCESS_KEY_ID", "x")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "y")
	_, err := NewS3StoreFromEnv(context.Background(), S3FromEnvOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNewS3StoreFromEnvBuildsClient(t *testing.T) {
	t.Setenv("TRAJIR_S3_BUCKET", "trajir")
	t.Setenv("TRAJIR_S3_ENDPOINT_URL", "http://127.0.0.1:9000")
	t.Setenv("AWS_ACCESS_KEY_ID", "minioadmin")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "minioadmin")
	t.Setenv("AWS_REGION", "us-east-1")
	store, err := NewS3StoreFromEnv(context.Background(), S3FromEnvOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if store.Bucket != "trajir" {
		t.Fatalf("bucket=%s", store.Bucket)
	}
	if _, ok := store.Client.(*AWSObjectAPI); !ok {
		t.Fatalf("client type %T", store.Client)
	}
}

func TestAWSObjectAPINilClient(t *testing.T) {
	api := &AWSObjectAPI{}
	if err := api.PutObject("b", "k", []byte("x")); err == nil {
		t.Fatal("expected error")
	}
}
