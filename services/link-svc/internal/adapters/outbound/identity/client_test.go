package identity_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/duykhanh/worklane/services/link-svc/internal/adapters/outbound/identity"
	"github.com/duykhanh/worklane/services/link-svc/internal/app"
)

func TestClient_Introspect_Active(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Internal-Token") == "" {
			t.Error("missing X-Internal-Token header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"active":true,"tenant_id":"t1"}`))
	}))
	defer srv.Close()

	c := identity.NewClient(srv.URL, "tok")
	tenant, err := c.Introspect(context.Background(), "my-api-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tenant != "t1" {
		t.Fatalf("want tenant t1, got %q", tenant)
	}
}

func TestClient_Introspect_Inactive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"active":false}`))
	}))
	defer srv.Close()

	c := identity.NewClient(srv.URL, "tok")
	_, err := c.Introspect(context.Background(), "revoked-key")
	if !errors.Is(err, app.ErrInvalidAPIKey) {
		t.Fatalf("want ErrInvalidAPIKey, got %v", err)
	}
}

func TestClient_Introspect_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := identity.NewClient(srv.URL, "tok")
	_, err := c.Introspect(context.Background(), "any-key")
	if err == nil {
		t.Fatal("expected error on 500")
	}
	if errors.Is(err, app.ErrInvalidAPIKey) {
		t.Fatal("500 should not be ErrInvalidAPIKey")
	}
}
