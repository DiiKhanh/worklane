package resendmail_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/duykhanh/worklane/services/otp-dispatcher/internal/adapters/outbound/resendmail"
)

func TestSend_PostsToResendAndReturnsID(t *testing.T) {
	var gotAuth, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "msg-123"})
	}))
	defer srv.Close()

	p := resendmail.New("re_test_key", "otp@worklane.dev", srv.URL, srv.Client())
	id, err := p.Send(context.Background(), "user@example.com", "Your code", "Code: 123456")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if id != "msg-123" {
		t.Fatalf("want provider id msg-123, got %q", id)
	}
	if gotPath != "/emails" {
		t.Fatalf("want POST /emails, got path %q", gotPath)
	}
	if gotAuth != "Bearer re_test_key" {
		t.Fatalf("want bearer auth, got %q", gotAuth)
	}
	if !strings.Contains(gotBody, "user@example.com") || !strings.Contains(gotBody, "Your code") {
		t.Fatalf("body missing recipient/subject: %s", gotBody)
	}
}

func TestSend_ErrorOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"invalid api key"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	p := resendmail.New("bad", "otp@worklane.dev", srv.URL, srv.Client())
	if _, err := p.Send(context.Background(), "user@example.com", "s", "b"); err == nil {
		t.Fatal("non-2xx response must return an error")
	}
}
