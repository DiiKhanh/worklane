package domain

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalid     = errors.New("link: invalid click")
	ErrUnavailable = errors.New("link: persistence unavailable")
)

type Device string

const (
	DeviceDesktop Device = "desktop"
	DeviceMobile  Device = "mobile"
	DeviceTablet  Device = "tablet"
	DeviceUnknown Device = "unknown"
)

type Click struct {
	Code           string
	TenantID       string
	TS             time.Time
	ReferrerOrigin string
	Device         Device
}

func Validate(event Click, now time.Time) error {
	if len(event.TenantID) == 0 || len(event.TenantID) > 36 || !printableASCII(event.TenantID) {
		return ErrInvalid
	}
	if event.TS.IsZero() || event.TS.UTC().Year() < 1000 || event.TS.After(now) {
		return ErrInvalid
	}
	switch event.Device {
	case DeviceDesktop, DeviceMobile, DeviceTablet, DeviceUnknown:
	default:
		return ErrInvalid
	}
	if event.ReferrerOrigin == "" {
		return nil
	}
	if len(event.ReferrerOrigin) > 512 || !printableASCII(event.ReferrerOrigin) || strings.Contains(event.ReferrerOrigin, "\\") {
		return ErrInvalid
	}
	u, err := url.Parse(event.ReferrerOrigin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Opaque != "" || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" || strings.HasSuffix(u.Host, ":") {
		return ErrInvalid
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return ErrInvalid
		}
	}
	if u.Scheme+"://"+u.Host != event.ReferrerOrigin {
		return ErrInvalid
	}
	return nil
}

func printableASCII(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] < '!' || value[i] > '~' {
			return false
		}
	}
	return true
}
