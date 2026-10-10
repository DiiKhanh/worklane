package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestLoadNotificationSettings_DisabledWithoutDSN(t *testing.T) {
	t.Setenv("NOTIFICATION_MYSQL_DSN", "")
	t.Setenv("NOTIFICATION_MAX_ATTEMPTS", "0") // not validated while disabled
	s, err := loadNotificationSettings()
	if err != nil || s.enabled() {
		t.Fatalf("enabled=%v err=%v", s.enabled(), err)
	}
}

func TestLoadNotificationSettings_Defaults(t *testing.T) {
	t.Setenv("NOTIFICATION_MYSQL_DSN", "root:secret@tcp(mysql:3306)/notification?parseTime=true")
	s, err := loadNotificationSettings()
	if err != nil {
		t.Fatal(err)
	}
	if !s.enabled() || s.Topics["email"] != "notification.email.requested" || s.Topics["sms"] != "notification.sms.requested" ||
		s.Group != "notification-dispatcher" || s.SentTopic != "notification.sent" || s.FailedTopic != "notification.failed" ||
		s.DLQTopic != "notification.dlq" || s.MaxAttempts != 3 || s.BaseBackoff != 500*time.Millisecond {
		t.Fatalf("defaults: %+v", s)
	}
}

func TestLoadNotificationSettings_Overrides(t *testing.T) {
	t.Setenv("NOTIFICATION_MYSQL_DSN", "dsn")
	t.Setenv("KAFKA_TOPIC_NOTIFICATION_SMS", "n.sms")
	t.Setenv("NOTIFICATION_MAX_ATTEMPTS", "5")
	t.Setenv("NOTIFICATION_RETRY_BACKOFF", "2s")
	s, err := loadNotificationSettings()
	if err != nil {
		t.Fatal(err)
	}
	if s.Topics["sms"] != "n.sms" || s.MaxAttempts != 5 || s.BaseBackoff != 2*time.Second {
		t.Fatalf("overrides: %+v", s)
	}
}

func TestLoadNotificationSettings_RejectsBadValues(t *testing.T) {
	for env, tc := range map[string]struct{ value, want string }{
		"NOTIFICATION_MAX_ATTEMPTS":  {"0", "NOTIFICATION_MAX_ATTEMPTS"},
		"NOTIFICATION_RETRY_BACKOFF": {"-1s", "NOTIFICATION_RETRY_BACKOFF"},
	} {
		t.Run(env, func(t *testing.T) {
			t.Setenv("NOTIFICATION_MYSQL_DSN", "dsn")
			t.Setenv(env, tc.value)
			if _, err := loadNotificationSettings(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestSleepCtx(t *testing.T) {
	if err := sleepCtx(context.Background(), time.Millisecond); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sleepCtx(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}
