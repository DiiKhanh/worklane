package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// setRequired sets the two secrets that have no default, so each test can focus on
// the single knob it varies.
func setRequired(t *testing.T) {
	t.Helper()
	t.Setenv("AUTH_JWT_PUBLIC_KEY", "pem")
	t.Setenv("INTERNAL_API_TOKEN", "token")
}

func TestLoadSettingsDefaults(t *testing.T) {
	setRequired(t)

	s, err := loadSettings()
	if err != nil {
		t.Fatalf("loadSettings: %v", err)
	}
	if s.HTTPAddr != ":8891" {
		t.Errorf("HTTPAddr = %q, want :8891", s.HTTPAddr)
	}
	if s.MigrationsDir != "db/notification/migrations" {
		t.Errorf("MigrationsDir = %q", s.MigrationsDir)
	}
	if !strings.Contains(s.MySQLDSN, "/notification?") {
		t.Errorf("MySQLDSN = %q, want the notification database", s.MySQLDSN)
	}
	if s.App.EmailTopic != "notification.email.requested" {
		t.Errorf("EmailTopic = %q, want notification.email.requested", s.App.EmailTopic)
	}
	if s.App.SMSTopic != "notification.sms.requested" {
		t.Errorf("SMSTopic = %q, want notification.sms.requested", s.App.SMSTopic)
	}
	if s.App.TenantLimitMax <= 0 || s.App.TenantLimitWindow <= 0 {
		t.Errorf("tenant limit = %d per %v, want positive", s.App.TenantLimitMax, s.App.TenantLimitWindow)
	}
	if s.App.UserLimitMax <= 0 || s.App.UserLimitWindow <= 0 {
		t.Errorf("user limit = %d per %v, want positive", s.App.UserLimitMax, s.App.UserLimitWindow)
	}
	if s.App.ListLimit <= 0 {
		t.Errorf("ListLimit = %d, want positive", s.App.ListLimit)
	}
}

func TestLoadSettingsOverrides(t *testing.T) {
	setRequired(t)
	t.Setenv("HTTP_ADDR", ":9000")
	t.Setenv("KAFKA_TOPIC_NOTIFICATION_EMAIL", "mail")
	t.Setenv("KAFKA_TOPIC_NOTIFICATION_SMS", "text")
	t.Setenv("NOTIFICATION_LIMIT_TENANT", "7")
	t.Setenv("NOTIFICATION_WINDOW_TENANT", "10m")
	t.Setenv("NOTIFICATION_LIMIT_USER", "3")
	t.Setenv("NOTIFICATION_WINDOW_USER", "24h")
	t.Setenv("NOTIFICATION_LIST_LIMIT", "20")
	t.Setenv("KAFKA_BROKERS", "a:9092, b:9092")

	s, err := loadSettings()
	if err != nil {
		t.Fatalf("loadSettings: %v", err)
	}
	if s.HTTPAddr != ":9000" {
		t.Errorf("HTTPAddr = %q, want :9000", s.HTTPAddr)
	}
	if s.App.EmailTopic != "mail" || s.App.SMSTopic != "text" {
		t.Errorf("topics = %q / %q, want mail / text", s.App.EmailTopic, s.App.SMSTopic)
	}
	if s.App.TenantLimitMax != 7 || s.App.TenantLimitWindow != 10*time.Minute {
		t.Errorf("tenant limit = %d per %v, want 7 per 10m", s.App.TenantLimitMax, s.App.TenantLimitWindow)
	}
	if s.App.UserLimitMax != 3 || s.App.UserLimitWindow != 24*time.Hour {
		t.Errorf("user limit = %d per %v, want 3 per 24h", s.App.UserLimitMax, s.App.UserLimitWindow)
	}
	if s.App.ListLimit != 20 {
		t.Errorf("ListLimit = %d, want 20", s.App.ListLimit)
	}
	if len(s.Brokers) != 2 || s.Brokers[1] != "b:9092" {
		t.Errorf("Brokers = %v", s.Brokers)
	}
}

func TestLoadSettingsReadsSecretsFromFiles(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "jwt.pub")
	if err := os.WriteFile(keyPath, []byte("pem-from-file"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AUTH_JWT_PUBLIC_KEY_FILE", keyPath)
	t.Setenv("INTERNAL_API_TOKEN", "token")

	s, err := loadSettings()
	if err != nil {
		t.Fatalf("loadSettings: %v", err)
	}
	if s.JWTPublicKey != "pem-from-file" {
		t.Errorf("JWTPublicKey = %q, want the file content", s.JWTPublicKey)
	}
}

func TestLoadSettingsRejectsInvalid(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{"missing jwt key", map[string]string{"AUTH_JWT_PUBLIC_KEY": ""}, "AUTH_JWT_PUBLIC_KEY"},
		{"missing internal token", map[string]string{"INTERNAL_API_TOKEN": ""}, "INTERNAL_API_TOKEN"},
		{"zero tenant limit", map[string]string{"NOTIFICATION_LIMIT_TENANT": "0"}, "NOTIFICATION_LIMIT_TENANT"},
		{"zero tenant window", map[string]string{"NOTIFICATION_WINDOW_TENANT": "0s"}, "NOTIFICATION_WINDOW_TENANT"},
		{"negative tenant window", map[string]string{"NOTIFICATION_WINDOW_TENANT": "-1m"}, "NOTIFICATION_WINDOW_TENANT"},
		{"zero user limit", map[string]string{"NOTIFICATION_LIMIT_USER": "0"}, "NOTIFICATION_LIMIT_USER"},
		{"zero user window", map[string]string{"NOTIFICATION_WINDOW_USER": "0s"}, "NOTIFICATION_WINDOW_USER"},
		{"zero list limit", map[string]string{"NOTIFICATION_LIST_LIMIT": "0"}, "NOTIFICATION_LIST_LIMIT"},
		{"negative list limit", map[string]string{"NOTIFICATION_LIST_LIMIT": "-5"}, "NOTIFICATION_LIST_LIMIT"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setRequired(t)
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			_, err := loadSettings()
			if err == nil {
				t.Fatal("loadSettings succeeded, want an error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to name %s", err, tt.wantErr)
			}
		})
	}
}

func TestComposedAdapters(t *testing.T) {
	a, b := uuidGen{}.New(), uuidGen{}.New()
	if len(a) != 36 || a == b {
		t.Errorf("uuidGen.New() = %q, %q, want distinct 36-char ids", a, b)
	}
	if now := (realClock{}).Now(); now.IsZero() {
		t.Error("realClock.Now() is zero")
	}
}
