package cas

import (
	"bytes"
	"testing"
)

func TestS3PutGetRehydrate(t *testing.T) {
	api := NewMemoryObjectAPI()
	store, err := NewS3Store(api, "trajir", "")
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("s3 payload\n")
	h, err := store.Put(payload)
	if err != nil {
		t.Fatal(err)
	}
	if !store.Has(h) {
		t.Fatal("Has=false after Put")
	}
	got, err := store.Get(h)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("Get mismatch")
	}
	uri, err := store.URIFor(h)
	if err != nil {
		t.Fatal(err)
	}
	if uri != "s3://trajir/cas/"+h[:2]+"/"+h {
		t.Fatalf("URI=%s", uri)
	}

	rehyd, err := Rehydrate(store, []ArtifactEntry{{ContentHash: h, LogicalPath: "out.bin"}}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(rehyd[h], payload) {
		t.Fatal("rehydrate mismatch")
	}
}

func TestS3PutIdempotent(t *testing.T) {
	api := NewMemoryObjectAPI()
	store, err := NewS3Store(api, "b", "prefix")
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("once")
	h1, err := store.Put(data)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := store.Put(data)
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Fatal("hash changed")
	}
	key, err := store.ObjectKey(h1)
	if err != nil {
		t.Fatal(err)
	}
	if key != "prefix/cas/"+h1[:2]+"/"+h1 {
		t.Fatalf("key=%s", key)
	}
}

func TestS3GetMissing(t *testing.T) {
	api := NewMemoryObjectAPI()
	store, err := NewS3Store(api, "b", "")
	if err != nil {
		t.Fatal(err)
	}
	h := ContentHash([]byte("missing"))
	_, err = store.Get(h)
	if err == nil {
		t.Fatal("expected not found")
	}
}

func TestS3RequiresBucket(t *testing.T) {
	_, err := NewS3Store(NewMemoryObjectAPI(), "", "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestS3IntegrityOnGet(t *testing.T) {
	api := NewMemoryObjectAPI()
	store, err := NewS3Store(api, "b", "")
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("good")
	h, err := store.Put(data)
	if err != nil {
		t.Fatal(err)
	}
	key, _ := store.ObjectKey(h)
	// Corrupt stored bytes while keeping the key.
	api.Objects["b/"+key] = []byte("tampered")
	_, err = store.Get(h)
	if err == nil {
		t.Fatal("expected integrity error")
	}
}
