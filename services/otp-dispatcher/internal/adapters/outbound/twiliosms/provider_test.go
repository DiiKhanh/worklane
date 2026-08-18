package twiliosms_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/duykhanh/worklane/services/otp-dispatcher/internal/adapters/outbound/twiliosms"
)

func TestSend_PostsFormAndReturnsSID(t *testing.T) {
	var gotAuth, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sid":"SM123"}`))
	}))
	defer srv.Close()

	p := twiliosms.New("ACxxx", "tok", "+15005550006", srv.URL, srv.Client())
	id, err := p.Send(context.Background(), "+84901234567", "Your code is 123456")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if id != "SM123" {
		t.Fatalf("want sid SM123, got %q", id)
	}
	if gotPath != "/2010-04-01/Accounts/ACxxx/Messages.json" {
		t.Fatalf("unexpected path %q", gotPath)
	}
	if !strings.HasPrefix(gotAuth, "Basic ") {
		t.Fatalf("want basic auth, got %q", gotAuth)
	}
	if !strings.Contains(gotBody, "To=%2B84901234567") || !strings.Contains(gotBody, "From=%2B15005550006") {
		t.Fatalf("form body missing To/From: %s", gotBody)
	}
}

func TestSend_ErrorOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"auth failed"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	p := twiliosms.New("AC", "bad", "+1", srv.URL, srv.Client())
	if _, err := p.Send(context.Background(), "+84901234567", "b"); err == nil {
		t.Fatal("non-2xx must return an error")
	}
}
