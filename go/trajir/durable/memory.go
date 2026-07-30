package durable

import (
	"context"
	"sync"
)

// Memory is an in process Backend. Fine for unit tests. Does not survive process exit.
type Memory struct {
	mu   sync.Mutex
	data map[string][]byte
}

// NewMemory returns an empty Memory backend.
func NewMemory() *Memory {
	return &Memory{data: make(map[string][]byte)}
}

func memKey(workflowID, stepKey string) string {
	return workflowID + "\x00" + stepKey
}

// Load implements Backend.
func (m *Memory) Load(_ context.Context, workflowID, stepKey string) ([]byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.data[memKey(workflowID, stepKey)]
	if !ok {
		return nil, false, nil
	}
	out := make([]byte, len(v))
	copy(out, v)
	return out, true, nil
}

// Save implements Backend. First writer wins.
func (m *Memory) Save(_ context.Context, workflowID, stepKey string, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := memKey(workflowID, stepKey)
	if _, exists := m.data[k]; exists {
		return nil
	}
	cp := make([]byte, len(value))
	copy(cp, value)
	m.data[k] = cp
	return nil
}

// Close implements Backend.
func (m *Memory) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data = nil
	return nil
}
