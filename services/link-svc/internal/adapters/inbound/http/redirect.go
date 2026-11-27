package http

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"

	"github.com/duykhanh/worklane/pkg/contracts/link"
	"github.com/duykhanh/worklane/services/link-svc/internal/domain"
)

const maxReferrerOriginBytes = 512

func redirect(resolver Resolution, clicks ClickEmitter) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		code := c.Param("code")
		if !validCode(code) {
			c.Status(http.StatusNotFound)
			return
		}
		mapping, err := resolver.Resolve(c.Request.Context(), code)
		switch {
		case errors.Is(err, domain.ErrNotFound):
			c.Status(http.StatusNotFound)
		case err != nil:
			c.Status(http.StatusServiceUnavailable)
		case mapping.DisabledAt != nil:
			c.Status(http.StatusGone)
		default:
			// Avoid redirect helpers that normalize Location or generate HTML.
			c.Header("Location", mapping.LongURL)
			c.Status(http.StatusFound)
			c.Writer.WriteHeaderNow()
			if c.Request.Method == http.MethodGet && clicks != nil {
				clicks.TryEmit(link.ClickedEvent{Code: mapping.Code, TenantID: mapping.TenantID, TS: time.Now().UTC(),
					ReferrerOrigin: referrerOrigin(c.Request.Referer()), Device: deviceCategory(c.Request.UserAgent())})
			}
		}
	}
}

func referrerOrigin(raw string) string {
	if len(raw) > MaxDestinationBytes || strings.ContainsAny(raw, "\\") || strings.IndexFunc(raw, unicode.IsControl) >= 0 {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.Opaque != "" {
		return ""
	}
	if strings.HasSuffix(u.Host, ":") {
		return ""
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return ""
		}
	}
	origin := u.Scheme + "://" + strings.ToLower(u.Host)
	// Unsupported origins become unknown so the dispatcher can retain the click.
	if len(origin) > maxReferrerOriginBytes {
		return ""
	}
	for _, char := range origin {
		if char < '!' || char > '~' {
			return ""
		}
	}
	return origin
}

func deviceCategory(raw string) link.Device {
	if len(raw) > MaxDestinationBytes {
		return link.DeviceUnknown
	}
	ua := strings.ToLower(raw)
	switch {
	case strings.Contains(ua, "bot"), strings.Contains(ua, "crawler"), strings.Contains(ua, "spider"):
		return link.DeviceUnknown
	case strings.Contains(ua, "ipad"), strings.Contains(ua, "tablet"), strings.Contains(ua, "android") && !strings.Contains(ua, "mobile"):
		return link.DeviceTablet
	case strings.Contains(ua, "mobile"), strings.Contains(ua, "iphone"):
		return link.DeviceMobile
	case strings.Contains(ua, "mozilla/"), strings.Contains(ua, "windows"), strings.Contains(ua, "macintosh"), strings.Contains(ua, "x11"):
		return link.DeviceDesktop
	default:
		return link.DeviceUnknown
	}
}
