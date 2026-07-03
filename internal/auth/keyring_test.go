package auth

import (
	"testing"
)

func TestMockKeyring_RoundTrip(t *testing.T) {
	m := NewMockKeyring()
	if err := m.Set("svc", "k", "v"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := m.Get("svc", "k")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "v" {
		t.Errorf("Get = %q, want v", got)
	}
	if err := m.Delete("svc", "k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := m.Get("svc", "k"); err == nil {
		t.Error("Get after Delete should fail")
	}
}

func TestMockKeyring_NotFound(t *testing.T) {
	m := NewMockKeyring()
	_, err := m.Get("svc", "missing")
	if err != ErrNotFound {
		t.Errorf("Get missing = %v, want ErrNotFound", err)
	}
}

func TestMockKeyring_Unavailable(t *testing.T) {
	m := NewMockKeyring()
	m.Unavailable = true
	if _, err := m.Get("svc", "k"); err != ErrKeyringUnavailable {
		t.Errorf("Get when unavailable = %v, want ErrKeyringUnavailable", err)
	}
	if err := m.Set("svc", "k", "v"); err != ErrKeyringUnavailable {
		t.Errorf("Set when unavailable = %v, want ErrKeyringUnavailable", err)
	}
	if err := m.Delete("svc", "k"); err != ErrKeyringUnavailable {
		t.Errorf("Delete when unavailable = %v, want ErrKeyringUnavailable", err)
	}
}