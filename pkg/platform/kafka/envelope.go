// Package kafka wraps IBM/sarama with the project's conventions: a typed envelope with
// a discriminator, config-driven topics (passed in, never hard-coded), and simple
// producer/consumer helpers. Infrastructure (pkg): adapters and composition roots use
// it; domain and app do not.
package kafka

import (
	"encoding/json"
	"fmt"
)

// Envelope is the on-the-wire shape of every message. MsgType is a discriminator so a
// consumer that handles several event types can switch on it before decoding Data.
type Envelope struct {
	MsgType string          `json:"msg_type"`
	Data    json.RawMessage `json:"data"`
}

// Wrap marshals data and boxes it in an Envelope, returning the bytes to publish. The
// msgType is derived from the concrete Go type (e.g. "otp.RequestedEvent").
func Wrap(data any) ([]byte, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("kafka: marshal data: %w", err)
	}
	env := Envelope{MsgType: fmt.Sprintf("%T", data), Data: raw}
	b, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("kafka: marshal envelope: %w", err)
	}
	return b, nil
}

// Unwrap parses envelope bytes and decodes the inner Data into dst.
func Unwrap(b []byte, dst any) (msgType string, err error) {
	var env Envelope
	if err := json.Unmarshal(b, &env); err != nil {
		return "", fmt.Errorf("kafka: unmarshal envelope: %w", err)
	}
	if err := json.Unmarshal(env.Data, dst); err != nil {
		return env.MsgType, fmt.Errorf("kafka: unmarshal data: %w", err)
	}
	return env.MsgType, nil
}
