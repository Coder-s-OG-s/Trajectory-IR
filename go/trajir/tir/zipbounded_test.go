package tir

import (
	"archive/zip"
	"bytes"
	"errors"
	"strings"
	"testing"
)

func zipWith(t *testing.T, name string, data []byte) *zip.File {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	return zr.File[0]
}

func TestReadZipMemberBoundedChargesBudget(t *testing.T) {
	f := zipWith(t, "a.json", []byte(`{"a":1}`))
	budget := 100
	got, err := readZipMemberBounded(f, &budget)
	if err != nil || string(got) != `{"a":1}` {
		t.Fatalf("got=%q err=%v", got, err)
	}
	if budget != 100-len(got) {
		t.Fatalf("budget=%d, want %d", budget, 100-len(got))
	}
}

func TestReadZipMemberBoundedRejectsOverBudgetWithoutReadingItAll(t *testing.T) {
	f := zipWith(t, "big.bin", bytes.Repeat([]byte("x"), 4096))
	budget := 1000
	_, err := readZipMemberBounded(f, &budget)
	if !errors.Is(err, ErrLimit) || !strings.Contains(err.Error(), "total uncompressed size") {
		t.Fatalf("expected a total-budget ErrLimit, got: %v", err)
	}
	if budget != 1000 {
		t.Fatalf("a rejected read must not consume budget, got %d", budget)
	}
}

func TestReadZipMemberBoundedAcceptsExactBudget(t *testing.T) {
	f := zipWith(t, "x.bin", bytes.Repeat([]byte("y"), 64))
	budget := 64
	got, err := readZipMemberBounded(f, &budget)
	if err != nil || len(got) != 64 || budget != 0 {
		t.Fatalf("len=%d budget=%d err=%v", len(got), budget, err)
	}
}
