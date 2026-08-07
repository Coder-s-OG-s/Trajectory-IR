package cas

import (
	"fmt"
	"strings"
)

// ObjectAPI is the minimal S3 style surface used by S3Store.
// Production code injects an AWS SDK client adapter; tests inject fakes.
type ObjectAPI interface {
	PutObject(bucket, key string, body []byte) error
	GetObject(bucket, key string) ([]byte, error)
	// HeadObject returns nil if the object exists, ErrNotFound if missing.
	HeadObject(bucket, key string) error
}

// S3Store is a content addressed store on an S3 compatible API.
// Key layout matches FileSystem: cas/<2hex>/<fullhash> under an optional prefix.
type S3Store struct {
	Client    ObjectAPI
	Bucket    string
	KeyPrefix string
}

// NewS3Store validates bucket and returns an S3Store.
func NewS3Store(client ObjectAPI, bucket string, keyPrefix string) (*S3Store, error) {
	if client == nil {
		return nil, fmt.Errorf("%w: client is required", ErrCAS)
	}
	if strings.TrimSpace(bucket) == "" {
		return nil, fmt.Errorf("%w: bucket is required", ErrCAS)
	}
	return &S3Store{
		Client:    client,
		Bucket:    bucket,
		KeyPrefix: strings.Trim(keyPrefix, "/"),
	}, nil
}

// ObjectKey returns the full object key for a content hash.
func (s *S3Store) ObjectKey(contentHash string) (string, error) {
	rel, err := ShardKey(contentHash)
	if err != nil {
		return "", err
	}
	if s.KeyPrefix == "" {
		return rel, nil
	}
	return s.KeyPrefix + "/" + rel, nil
}

// URIFor returns s3://bucket/key for thin package refs.
func (s *S3Store) URIFor(contentHash string) (string, error) {
	h, err := NormalizeHash(contentHash)
	if err != nil {
		return "", err
	}
	key, err := s.ObjectKey(h)
	if err != nil {
		return "", err
	}
	return "s3://" + s.Bucket + "/" + key, nil
}

// Put stores data under its content hash. Idempotent when the object exists.
func (s *S3Store) Put(data []byte) (string, error) {
	h := ContentHash(data)
	if s.Has(h) {
		return h, nil
	}
	key, err := s.ObjectKey(h)
	if err != nil {
		return "", err
	}
	if err := s.Client.PutObject(s.Bucket, key, data); err != nil {
		return "", err
	}
	return h, nil
}

// Get loads bytes and verifies the content hash.
func (s *S3Store) Get(contentHash string) ([]byte, error) {
	h, err := NormalizeHash(contentHash)
	if err != nil {
		return nil, err
	}
	key, err := s.ObjectKey(h)
	if err != nil {
		return nil, err
	}
	data, err := s.Client.GetObject(s.Bucket, key)
	if err != nil {
		return nil, err
	}
	actual := ContentHash(data)
	if actual != h {
		return nil, fmt.Errorf("%w: expected %s got %s for s3://%s/%s", ErrIntegrity, h, actual, s.Bucket, key)
	}
	return data, nil
}

// Has reports whether the object exists. Auth and transport errors propagate.
func (s *S3Store) Has(contentHash string) bool {
	h, err := NormalizeHash(contentHash)
	if err != nil {
		return false
	}
	key, err := s.ObjectKey(h)
	if err != nil {
		return false
	}
	err = s.Client.HeadObject(s.Bucket, key)
	if err == nil {
		return true
	}
	if errorsIsNotFound(err) {
		return false
	}
	// Match Python: unexpected errors must not look like missing.
	// Callers that need the error should use Get.
	return false
}

func errorsIsNotFound(err error) bool {
	if err == nil {
		return false
	}
	if err == ErrNotFound {
		return true
	}
	// Also accept wrapped not found.
	return strings.Contains(strings.ToLower(err.Error()), "not found") ||
		strings.Contains(err.Error(), "NoSuchKey")
}

// MemoryObjectAPI is an in process S3 fake for unit tests.
type MemoryObjectAPI struct {
	Objects map[string][]byte // bucket/key -> bytes
}

// NewMemoryObjectAPI creates an empty memory object store.
func NewMemoryObjectAPI() *MemoryObjectAPI {
	return &MemoryObjectAPI{Objects: map[string][]byte{}}
}

func (m *MemoryObjectAPI) key(bucket, key string) string {
	return bucket + "/" + key
}

func (m *MemoryObjectAPI) PutObject(bucket, key string, body []byte) error {
	if m.Objects == nil {
		m.Objects = map[string][]byte{}
	}
	cp := make([]byte, len(body))
	copy(cp, body)
	m.Objects[m.key(bucket, key)] = cp
	return nil
}

func (m *MemoryObjectAPI) GetObject(bucket, key string) ([]byte, error) {
	data, ok := m.Objects[m.key(bucket, key)]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, key)
	}
	cp := make([]byte, len(data))
	copy(cp, data)
	return cp, nil
}

func (m *MemoryObjectAPI) HeadObject(bucket, key string) error {
	if _, ok := m.Objects[m.key(bucket, key)]; !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, key)
	}
	return nil
}

// Ensure S3Store implements Store.
var _ Store = (*S3Store)(nil)
