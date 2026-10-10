//go:build e2e

package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// notifJSON calls notification-api through Traefik on its dedicated host and decodes the
// JSON body into out (skipped when out is nil). It returns the HTTP status.
func notifJSON(t *testing.T, method, path, key, body string, out any) int {
	t.Helper()
	req, err := http.NewRequest(method, apiBase+path, bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	req.Host = notifyHost
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("%s %s: read body: %v", method, path, err)
	}
	if out != nil {
		if err := json.Unmarshal(b, out); err != nil {
			t.Fatalf("%s %s: decode %q: %v", method, path, b, err)
		}
	}
	return resp.StatusCode
}

type rendered struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

type sendResult struct {
	NotificationID string `json:"notification_id"`
	State          string `json:"state"`
}

// createTemplate stores an email template with one {{name}} variable and returns its id.
func createTemplate(t *testing.T, key string) string {
	t.Helper()
	var tpl struct {
		ID string `json:"id"`
	}
	body := `{"name":"e2e welcome","channel":"email","locale":"en","subject":"Welcome {{name}}","body":"Hello {{name}}, your account is ready."}`
	if status := notifJSON(t, http.MethodPost, "/v1/templates", key, body, &tpl); status != http.StatusCreated {
		t.Fatalf("create template: want 201, got %d", status)
	}
	if tpl.ID == "" {
		t.Fatal("create template: want an id")
	}
	return tpl.ID
}

func sendNotification(t *testing.T, key, body string) (int, sendResult) {
	t.Helper()
	var res sendResult
	status := notifJSON(t, http.MethodPost, "/v1/notifications", key, body, &res)
	return status, res
}

// mailsTo returns the subject and body of every message MailHog captured for recipient.
func mailsTo(t *testing.T, recipient string) []rendered {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("%s/api/v2/search?kind=to&query=%s", mailhog, url.QueryEscape(recipient)))
	if err != nil {
		t.Fatalf("mailhog search: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out struct {
		Items []struct {
			Content struct {
				Headers map[string][]string
				Body    string
			} `json:"Content"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("mailhog search: decode: %v", err)
	}
	mails := make([]rendered, 0, len(out.Items))
	for _, it := range out.Items {
		mails = append(mails, rendered{
			Subject: strings.Join(it.Content.Headers["Subject"], ""),
			Body:    strings.TrimSpace(it.Content.Body),
		})
	}
	return mails
}

// waitForState polls the detail endpoint until the notification reaches want (the
// pipeline is notification-api -> Kafka -> otp-dispatcher -> provider -> MySQL).
func waitForState(t *testing.T, key, id, want string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	var d struct {
		State string `json:"state"`
		Error string `json:"error"`
	}
	for time.Now().Before(deadline) {
		if status := notifJSON(t, http.MethodGet, "/v1/notifications/"+id, key, "", &d); status != http.StatusOK {
			t.Fatalf("detail: want 200, got %d", status)
		}
		if d.State == want {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("want notification %s in state %s within timeout, got %s (error %q)", id, want, d.State, d.Error)
}

func TestE2E_NotificationSendMatchesPreviewAndDedups(t *testing.T) {
	key := seedKey(t)
	tplID := createTemplate(t, key)
	recipient := fmt.Sprintf("notif-%d@example.com", time.Now().UnixNano())

	var preview rendered
	if status := notifJSON(t, http.MethodPost, "/v1/templates/"+tplID+"/preview", key, `{"variables":{"name":"Ada"}}`, &preview); status != http.StatusOK {
		t.Fatalf("preview: want 200, got %d", status)
	}
	if preview.Subject != "Welcome Ada" || preview.Body != "Hello Ada, your account is ready." {
		t.Fatalf("preview: unexpected render %+v", preview)
	}

	body := fmt.Sprintf(
		`{"channel":"email","recipient":%q,"template_id":%q,"variables":{"name":"Ada"},"kind":"transactional","idempotency_key":%q}`,
		recipient, tplID, newID(),
	)
	status, first := sendNotification(t, key, body)
	if status != http.StatusAccepted || first.NotificationID == "" || first.State != "queued" {
		t.Fatalf("send: want 202 queued with an id, got %d %+v", status, first)
	}
	waitForState(t, key, first.NotificationID, "sent")

	// The delivered message goes through the same render path as the preview.
	mails := mailsTo(t, recipient)
	if len(mails) != 1 || mails[0] != preview {
		t.Fatalf("delivery: want one mail equal to the preview %+v, got %+v", preview, mails)
	}

	// Same idempotency key: the earlier notification is returned and nothing is sent again.
	status, again := sendNotification(t, key, body)
	if status != http.StatusOK || again.NotificationID != first.NotificationID {
		t.Fatalf("resend: want 200 with id %s, got %d %+v", first.NotificationID, status, again)
	}
	var list []struct {
		ID string `json:"id"`
	}
	if status := notifJSON(t, http.MethodGet, "/v1/notifications", key, "", &list); status != http.StatusOK {
		t.Fatalf("list: want 200, got %d", status)
	}
	if len(list) != 1 || list[0].ID != first.NotificationID {
		t.Fatalf("list: want the one notification, got %+v", list)
	}
	if mails := mailsTo(t, recipient); len(mails) != 1 {
		t.Fatalf("resend: want still one mail, got %d", len(mails))
	}
}

func TestE2E_NotificationMarketingOptOutSuppressed(t *testing.T) {
	key := seedKey(t)
	tplID := createTemplate(t, key)
	recipient := fmt.Sprintf("optout-%d@example.com", time.Now().UnixNano())

	// The default-host path route reaches notification-api too, so PUT goes through it.
	req, err := http.NewRequest(http.MethodPut, apiBase+"/v1/preferences",
		strings.NewReader(`{"user_ref":"user-1","channel":"email","enabled":false}`))
	if err != nil {
		t.Fatalf("PUT /v1/preferences: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /v1/preferences: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("opt out: want 200, got %d", resp.StatusCode)
	}

	send := func(kind string) (int, sendResult) {
		return sendNotification(t, key, fmt.Sprintf(
			`{"channel":"email","recipient":%q,"template_id":%q,"variables":{"name":"Ada"},"kind":%q,"user_ref":"user-1"}`,
			recipient, tplID, kind,
		))
	}

	status, marketing := send("marketing")
	if status != http.StatusAccepted || marketing.State != "suppressed" {
		t.Fatalf("marketing: want 202 suppressed, got %d %+v", status, marketing)
	}

	// A transactional send to the same opted-out user still delivers.
	status, transactional := send("transactional")
	if status != http.StatusAccepted || transactional.State != "queued" {
		t.Fatalf("transactional: want 202 queued, got %d %+v", status, transactional)
	}
	waitForState(t, key, transactional.NotificationID, "sent")

	// Checked after the transactional delivery, so a suppressed send that leaked into the
	// queue ahead of it would already have arrived.
	if mails := mailsTo(t, recipient); len(mails) != 1 {
		t.Fatalf("want only the transactional mail, got %d", len(mails))
	}
}

func TestE2E_NotificationRequiresCredential(t *testing.T) {
	for _, path := range []string{"/v1/notifications", "/v1/templates"} {
		if status := notifJSON(t, http.MethodGet, path, "", "", nil); status != http.StatusUnauthorized {
			t.Fatalf("GET %s without a key: want 401, got %d", path, status)
		}
	}
}

func TestE2E_NotificationIsTenantScoped(t *testing.T) {
	owner, other := seedKey(t), seedKey(t)
	tplID := createTemplate(t, owner)

	if status := notifJSON(t, http.MethodGet, "/v1/templates/"+tplID, other, "", nil); status != http.StatusNotFound {
		t.Fatalf("other tenant's template: want 404, got %d", status)
	}
	body := fmt.Sprintf(
		`{"channel":"email","recipient":"scoped@example.com","template_id":%q,"kind":"transactional"}`, tplID,
	)
	if status, _ := sendNotification(t, other, body); status != http.StatusNotFound {
		t.Fatalf("send with another tenant's template: want 404, got %d", status)
	}
}
