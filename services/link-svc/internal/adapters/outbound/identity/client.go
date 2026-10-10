// Package identity is the outbound adapter that resolves opaque API keys to tenant IDs
// by calling auth-svc's /internal/introspect endpoint.
package identity

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/duykhanh/worklane/services/link-svc/internal/app"
)

var _ app.Introspector = (*Client)(nil)

// Client calls auth-svc to introspect an opaque API key.
type Client struct {
	baseURL       string
	internalToken string
	hc            *http.Client
}

// NewClient creates an identity Client targeting the given auth-svc base URL.
func NewClient(baseURL, internalToken string) *Client {
	return &Client{
		baseURL:       strings.TrimRight(baseURL, "/"),
		internalToken: internalToken,
		hc:            &http.Client{Timeout: 3 * time.Second},
	}
}

// introspectRequest mirrors auth-svc's /internal/introspect contract, which binds the
// field as "token" (binding:"required"). It must stay "token" or the machine path 400s.
type introspectRequest struct {
	Token string `json:"token"`
}

type introspectResponse struct {
	Active   bool   `json:"active"`
	TenantID string `json:"tenant_id"`
}

// Introspect resolves an opaque API key to a tenant ID via auth-svc.
func (c *Client) Introspect(ctx context.Context, apiKey string) (string, error) {
	body, err := json.Marshal(introspectRequest{Token: apiKey})
	if err != nil {
		return "", fmt.Errorf("identity: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/internal/introspect", strings.NewReader(string(body)))
	if err != nil {
		return "", fmt.Errorf("identity: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", c.internalToken)

	resp, err := c.hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("identity: transport: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("identity: unexpected status %d", resp.StatusCode)
	}

	var out introspectResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("identity: decode: %w", err)
	}
	if !out.Active {
		return "", app.ErrInvalidAPIKey
	}
	return out.TenantID, nil
}
