package cas

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestContentHashEmpty(t *testing.T) {
	// Public SHA-256 empty string vector.
	want := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if got := ContentHash(nil); got != want {
		t.Fatalf("ContentHash(nil)=%s want %s", got, want)
	}
	if got := ContentHash([]byte{}); got != want {
		t.Fatalf("ContentHash(empty)=%s want %s", got, want)
	}
}

func TestShardKey(t *testing.T) {
	h := ContentHash([]byte("hello"))
	key, err := ShardKey(h)
	if err != nil {
		t.Fatal(err)
	}
	want := "cas/" + h[:2] + "/" + h
	if key != want {
		t.Fatalf("ShardKey=%s want %s", key, want)
	}
}

func TestPutGetRoundTrip(t *testing.T) {
	fs, err := NewFileSystem(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("trajectory artifact")
	h, err := fs.Put(data)
	if err != nil {
		t.Fatal(err)
	}
	if h != ContentHash(data) {
		t.Fatalf("hash mismatch")
	}
	if !fs.Has(h) {
		t.Fatal("Has=false after Put")
	}
	got, err := fs.Get(h)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("Get bytes mismatch")
	}
	uri, err := fs.URIFor(h)
	if err != nil {
		t.Fatal(err)
	}
	if uri != "cas://"+h {
		t.Fatalf("URI=%s", uri)
	}
	// sharded path exists
	path, err := fs.PathFor(h)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	if filepath.Base(filepath.Dir(path)) != h[:2] {
		t.Fatalf("shard dir = %s", filepath.Base(filepath.Dir(path)))
	}
}

func TestPutIdempotent(t *testing.T) {
	fs, err := NewFileSystem(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("same")
	h1, err := fs.Put(data)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := fs.Put(data)
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Fatal("idempotent put changed hash")
	}
}

func TestGetMissing(t *testing.T) {
	fs, err := NewFileSystem(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	h := ContentHash([]byte("absent"))
	_, err = fs.Get(h)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err=%v want ErrNotFound", err)
	}
	if fs.Has(h) {
		t.Fatal("Has should be false")
	}
}

func TestGetDetectsTamper(t *testing.T) {
	fs, err := NewFileSystem(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	h, err := fs.Put([]byte("honest"))
	if err != nil {
		t.Fatal(err)
	}
	path, err := fs.PathFor(h)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = fs.Get(h)
	if !errors.Is(err, ErrIntegrity) {
		t.Fatalf("err=%v want ErrIntegrity", err)
	}
}

func TestRehydrate(t *testing.T) {
	fs, err := NewFileSystem(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	h1, err := fs.Put([]byte("one"))
	if err != nil {
		t.Fatal(err)
	}
	h2 := ContentHash([]byte("missing"))
	entries := []ArtifactEntry{
		{LogicalPath: "a", ContentHash: h1},
		{LogicalPath: "b", ContentHash: h2},
	}
	_, err = Rehydrate(fs, entries, true)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("requireAll err=%v", err)
	}
	partial, err := Rehydrate(fs, entries, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(partial) != 1 || !bytes.Equal(partial[h1], []byte("one")) {
		t.Fatalf("partial=%v", partial)
	}
}

func TestNormalizeHashRejectsBad(t *testing.T) {
	if _, err := NormalizeHash("nope"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("err=%v", err)
	}
}
