//go:build integration

package smtpmail_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/duykhanh/worklane/services/otp-dispatcher/internal/adapters/outbound/smtpmail"
)

// TestSend_DeliversToMailHog proves the SMTP path end to end against a real MailHog:
// we send, then read it back from MailHog's HTTP API. This is exactly the mechanism the
// e2e test uses to read the OTP code.
func TestSend_DeliversToMailHog(t *testing.T) {
	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "mailhog/mailhog:v1.0.1",
		ExposedPorts: []string{"1025/tcp", "8025/tcp"},
		WaitingFor:   wait.ForListeningPort("8025/tcp"),
	}
	ctr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req, Started: true,
	})
	if err != nil {
		t.Fatalf("start mailhog: %v", err)
	}
	t.Cleanup(func() { _ = ctr.Terminate(ctx) })

	host, _ := ctr.Host(ctx)
	smtpPort, _ := ctr.MappedPort(ctx, "1025")
	apiPort, _ := ctr.MappedPort(ctx, "8025")

	port, _ := strconv.Atoi(smtpPort.Port())
	p := smtpmail.New(host, port, "otp@worklane.dev", "", "")
	if _, err := p.Send(ctx, "user@example.com", "Your code", "Your code is 123456"); err != nil {
		t.Fatalf("send: %v", err)
	}

	apiURL := fmt.Sprintf("http://%s:%s/api/v2/messages", host, apiPort.Port())
	resp, err := http.Get(apiURL)
	if err != nil {
		t.Fatalf("mailhog api: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "123456") || !strings.Contains(string(body), "user@example.com") {
		t.Fatalf("mailhog did not capture the message: %s", string(body))
	}
}
