// Package twiliosms is otp-dispatcher's SMS adapter: it sends text messages via the
// Twilio REST API and implements the dispatcher's app.SMSProvider port.
package twiliosms

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Provider talks to the Twilio Messages API. baseURL is injected (rather than hard-coded
// to https://api.twilio.com) so tests can point it at an httptest stub.
type Provider struct {
	accountSID string
	authToken  string
	from       string
	baseURL    string
	hc         *http.Client
}

func New(accountSID, authToken, from, baseURL string, hc *http.Client) *Provider {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &Provider{accountSID: accountSID, authToken: authToken, from: from, baseURL: baseURL, hc: hc}
}

type sendResponse struct {
	SID string `json:"sid"`
}

// Send delivers one SMS and returns the Twilio message SID. Any non-2xx response returns
// an error so the caller records a failed delivery.
func (p *Provider) Send(ctx context.Context, to, body string) (string, error) {
	form := url.Values{"To": {to}, "From": {p.from}, "Body": {body}}
	endpoint := fmt.Sprintf("%s/2010-04-01/Accounts/%s/Messages.json", p.baseURL, p.accountSID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("twilio: new request: %w", err)
	}
	req.SetBasicAuth(p.accountSID, p.authToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("twilio: do: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("twilio: status %d: %s", resp.StatusCode, string(respBody))
	}
	var out sendResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return "", fmt.Errorf("twilio: decode: %w", err)
	}
	return out.SID, nil
}
