package cas

import (
	"context"
	"os"
	"testing"
)

// Live MinIO / S3 path. Skips unless TRAJIR_S3_BUCKET and endpoint or real AWS env is set.
func TestLiveS3StoreFromEnvRoundTrip(t *testing.T) {
	if os.Getenv("TRAJIR_S3_BUCKET") == "" {
		t.Skip("set TRAJIR_S3_BUCKET (and usually TRAJIR_S3_ENDPOINT_URL) for live S3 CAS")
	}
	if os.Getenv("AWS_ACCESS_KEY_ID") == "" {
		t.Skip("set AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY for live S3 CAS")
	}

	store, err := NewS3StoreFromEnv(context.Background(), S3FromEnvOptions{})
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("trajir-live-s3-cas")
	h, err := store.Put(payload)
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(h)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("got %q", got)
	}
	if !store.Has(h) {
		t.Fatal("Has=false after Put")
	}
}
