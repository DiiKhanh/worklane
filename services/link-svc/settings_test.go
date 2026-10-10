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
	if s.NodeID != 0 {
		t.Errorf("NodeID = %d, want 0", s.NodeID)
	}
	if s.HTTPAddr != ":8890" {
		t.Errorf("HTTPAddr = %q, want :8890", s.HTTPAddr)
	}
	if s.MigrationsDir != "db/link/migrations" {
		t.Errorf("MigrationsDir = %q", s.MigrationsDir)
	}
	if !strings.Contains(s.MySQLDSN, "/link?") {
		t.Errorf("MySQLDSN = %q, want the link database", s.MySQLDSN)
	}
	if s.App.ClickedTopic != "link.clicked" {
		t.Errorf("ClickedTopic = %q, want link.clicked", s.App.ClickedTopic)
	}
	if s.App.CacheTTL != time.Hour {
		t.Errorf("CacheTTL = %v, want 1h", s.App.CacheTTL)
	}
	if s.App.CreateLimitMax <= 0 || s.App.CreateLimitWindow <= 0 {
		t.Errorf("create limit = %d per %v, want positive", s.App.CreateLimitMax, s.App.CreateLimitWindow)
	}
}

func TestLoadSettingsOverrides(t *testing.T) {
	setRequired(t)
	t.Setenv("LINK_NODE_ID", "1023")
	t.Setenv("LINK_PUBLIC_BASE", "https://link.example.com")
	t.Setenv("LINK_CACHE_TTL", "30m")
	t.Setenv("LINK_LIMIT_TENANT", "7")
	t.Setenv("LINK_WINDOW_TENANT", "10m")
	t.Setenv("KAFKA_TOPIC_LINK_CLICKED", "clicks")
	t.Setenv("KAFKA_BROKERS", "a:9092, b:9092")

	s, err := loadSettings()
	if err != nil {
		t.Fatalf("loadSettings: %v", err)
	}
	if s.NodeID != 1023 {
		t.Errorf("NodeID = %d, want 1023", s.NodeID)
	}
	if s.App.PublicBase != "https://link.example.com" {
		t.Errorf("PublicBase = %q", s.App.PublicBase)
	}
	if s.App.CacheTTL != 30*time.Minute {
		t.Errorf("CacheTTL = %v, want 30m", s.App.CacheTTL)
	}
	if s.App.CreateLimitMax != 7 || s.App.CreateLimitWindow != 10*time.Minute {
		t.Errorf("create limit = %d per %v, want 7 per 10m", s.App.CreateLimitMax, s.App.CreateLimitWindow)
	}
	if s.App.ClickedTopic != "clicks" {
		t.Errorf("ClickedTopic = %q, want clicks", s.App.ClickedTopic)
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
		{"node id not a number", map[string]string{"LINK_NODE_ID": "one"}, "LINK_NODE_ID"},
		{"node id negative", map[string]string{"LINK_NODE_ID": "-1"}, "LINK_NODE_ID"},
		{"node id too large", map[string]string{"LINK_NODE_ID": "1024"}, "LINK_NODE_ID"},
		{"missing jwt key", map[string]string{"AUTH_JWT_PUBLIC_KEY": ""}, "AUTH_JWT_PUBLIC_KEY"},
		{"missing internal token", map[string]string{"INTERNAL_API_TOKEN": ""}, "INTERNAL_API_TOKEN"},
		{"relative public base", map[string]string{"LINK_PUBLIC_BASE": "link.example.com"}, "LINK_PUBLIC_BASE"},
		{"non-http public base", map[string]string{"LINK_PUBLIC_BASE": "ftp://link.example.com"}, "LINK_PUBLIC_BASE"},
		{"zero cache ttl", map[string]string{"LINK_CACHE_TTL": "0s"}, "LINK_CACHE_TTL"},
		{"zero limit window", map[string]string{"LINK_WINDOW_TENANT": "0s"}, "LINK_WINDOW_TENANT"},
		{"negative limit window", map[string]string{"LINK_WINDOW_TENANT": "-1m"}, "LINK_WINDOW_TENANT"},
		{"zero limit max", map[string]string{"LINK_LIMIT_TENANT": "0"}, "LINK_LIMIT_TENANT"},
		{"zero inflight clicks", map[string]string{"LINK_MAX_INFLIGHT_CLICKS": "0"}, "LINK_MAX_INFLIGHT_CLICKS"},
		{"zero publish timeout", map[string]string{"LINK_CLICK_PUBLISH_TIMEOUT": "0s"}, "LINK_CLICK_PUBLISH_TIMEOUT"},
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
