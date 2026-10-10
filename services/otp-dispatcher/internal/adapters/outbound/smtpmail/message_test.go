package smtpmail

import (
	"errors"
	"fmt"
	"net/textproto"
	"strings"
	"testing"

	"github.com/duykhanh/worklane/services/otp-dispatcher/internal/app"
)

func TestBuildMessage(t *testing.T) {
	got := string(buildMessage("otp@worklane.dev", "user@example.com", "Your code", "Line 1\nLine 2"))
	want := "From: otp@worklane.dev\r\n" +
		"To: user@example.com\r\n" +
		"Subject: Your code\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"\r\n" +
		"Line 1\nLine 2"
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

// A header value must never be able to end its own line: that would let message data
// add headers or start the body early.
func TestBuildMessageKeepsHeaderValuesOnOneLine(t *testing.T) {
	got := string(buildMessage("otp@worklane.dev", "user@example.com\r\nBcc: a@evil.test",
		"Hi\r\nBcc: b@evil.test\n\nfake body", "real body"))
	head, body, ok := strings.Cut(got, "\r\n\r\n")
	if !ok || body != "real body" {
		t.Fatalf("the body must start at the first blank line, got %q", got)
	}
	lines := strings.Split(head, "\r\n")
	if len(lines) != 5 {
		t.Fatalf("want exactly 5 header lines, got %d: %q", len(lines), lines)
	}
	if lines[1] != "To: user@example.com Bcc: a@evil.test" || lines[2] != "Subject: Hi Bcc: b@evil.test fake body" {
		t.Fatalf("line breaks must collapse to a space, got %q", lines)
	}
}

func TestClassify(t *testing.T) {
	rejected := &textproto.Error{Code: 550, Msg: "mailbox unavailable"}
	busy := &textproto.Error{Code: 451, Msg: "try again later"}
	cases := []struct {
		name      string
		err       error
		permanent bool
	}{
		{"5yz reply is a definitive rejection", rejected, true},
		{"wrapped 5yz reply", fmt.Errorf("rcpt: %w", rejected), true},
		{"4yz reply asks to retry", busy, false},
		{"network error", errors.New("dial tcp: connection refused"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classify(tc.err)
			var perm *app.PermanentError
			if errors.As(got, &perm) != tc.permanent {
				t.Fatalf("permanent = %v, want %v", !tc.permanent, tc.permanent)
			}
			if !errors.Is(got, tc.err) {
				t.Fatalf("the cause must stay in the chain, got %v", got)
			}
			if want := "smtp: send: " + tc.err.Error(); got.Error() != want {
				t.Fatalf("message = %q, want %q", got.Error(), want)
			}
		})
	}
}
