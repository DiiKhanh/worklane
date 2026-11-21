package domain

import (
	"errors"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/duykhanh/worklane/pkg/contracts/link"
	"github.com/duykhanh/worklane/pkg/idgen"
)

var (
	ErrInvalid     = errors.New("link: invalid click")
	ErrUnavailable = errors.New("link: persistence unavailable")
)

func Validate(event link.ClickedEvent, now time.Time) error {
	id, err := idgen.Decode(event.Code)
	if err != nil || id > math.MaxInt64 || len(event.TenantID) == 0 || len(event.TenantID) > 36 || !printableASCII(event.TenantID) {
		return ErrInvalid
	}
	if event.TS.IsZero() || event.TS.UTC().Year() < 1000 || event.TS.After(now) {
		return ErrInvalid
	}
	switch event.Device {
	case link.DeviceDesktop, link.DeviceMobile, link.DeviceTablet, link.DeviceUnknown:
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
