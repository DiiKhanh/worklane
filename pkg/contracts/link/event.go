// Package link holds the infrastructure-free vocabulary and link.clicked payload
// shared by link-svc and link-dispatcher.
package link

import "time"

const DetailRetention = 30 * 24 * time.Hour

type State string

const (
	StateActive   State = "active"
	StateDisabled State = "disabled"
)

type Device string

const (
	DeviceDesktop Device = "desktop"
	DeviceMobile  Device = "mobile"
	DeviceTablet  Device = "tablet"
	DeviceUnknown Device = "unknown"
)

// ClickedEvent describes an active GET that returned 302. TS is event time in
// UTC. ReferrerOrigin is an HTTP/HTTPS origin only (no credentials, path, query
// or fragment), at most 512 visible ASCII bytes, or empty for unknown.
// Producers map unsupported origins to unknown before queue admission;
// consumers validate before persistence. Raw request metadata is never included.
type ClickedEvent struct {
	Code           string    `json:"code"`
	TenantID       string    `json:"tenant_id"`
	TS             time.Time `json:"ts"`
	ReferrerOrigin string    `json:"referrer_origin"`
	Device         Device    `json:"device"`
}

func (e ClickedEvent) PartitionKey() string { return e.Code }
