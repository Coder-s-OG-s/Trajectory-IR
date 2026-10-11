package tir

import (
	"errors"
	"testing"
)

func TestVerifySingleTenantEmptyThenAcme(t *testing.T) {
	err := verifySingleTenant(map[string]any{}, []map[string]any{
		{"tenant_id": ""},
		{"tenant_id": "acme"},
	})
	if !errors.Is(err, ErrVerification) {
		t.Fatalf("err=%v want ErrVerification", err)
	}
}

func TestVerifySingleTenantEmptyStringConsistent(t *testing.T) {
	err := verifySingleTenant(map[string]any{}, []map[string]any{
		{"tenant_id": ""},
		{"tenant_id": ""},
	})
	if err != nil {
		t.Fatalf("same empty tenant_id should pass: %v", err)
	}
}
