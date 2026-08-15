// Package resendmail is otp-dispatcher's email adapter: it sends transactional email
// via the Resend HTTP API and implements the dispatcher's EmailProvider port.
package resendmail

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Provider talks to the Resend REST API. baseURL is injected (rather than hard-coded to
// https://api.resend.com) so tests can point it at an httptest stub - the standard way
// to unit-test an HTTP client without hitting the network.
type Provider struct {
	apiKey  string
	from    string
	baseURL string
	hc      *http.Client
}

func New(apiKey, from, baseURL string, hc *http.Client) *Provider {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &Provider{apiKey: apiKey, from: from, baseURL: baseURL, hc: hc}
}

type sendRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Text    string   `json:"text"`
}

type sendResponse struct {
	ID string `json:"id"`
}

// Send delivers one email and returns the Resend message id. It returns an error on any
// non-2xx response so the caller (the dispatch handler) can record a failed delivery.
func (p *Provider) Send(ctx context.Context, to, subject, body string) (string, error) {
	payload, err := json.Marshal(sendRequest{From: p.from, To: []string{to}, Subject: subject, Text: body})
	if err != nil {
		return "", fmt.Errorf("resend: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/emails", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("resend: new request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("resend: do: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("resend: status %d: %s", resp.StatusCode, string(respBody))
	}
	var out sendResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return "", fmt.Errorf("resend: decode: %w", err)
	}
	return out.ID, nil
}
