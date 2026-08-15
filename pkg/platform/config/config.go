// Package config provides tiny typed helpers for reading environment variables with
// defaults. Infrastructure (pkg): used by composition roots (main.go), not by domain/app.
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Env returns the value of key, or def if unset/empty.
func Env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

// EnvInt parses key as an int, falling back to def on missing/invalid values.
func EnvInt(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// EnvDuration parses key as a Go duration (e.g. "5m"), falling back to def.
func EnvDuration(key string, def time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

// EnvList splits key on commas (e.g. broker lists), falling back to def.
func EnvList(key string, def []string) []string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		parts := strings.Split(v, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		return parts
	}
	return def
}
