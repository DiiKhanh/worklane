package domain

import "strings"

// Device labels returned by DeviceFromUA.
const (
	DeviceIOS     = "iOS"
	DeviceAndroid = "Android"
	DeviceWindows = "Windows"
	DeviceMacOS   = "macOS"
	DeviceLinux   = "Linux"
	DeviceOther   = "Other"
)

// DeviceFromUA is a deliberately simple user-agent classifier, applied at read time.
//
// Order matters: an iPhone UA contains "like Mac OS X" and an Android UA contains
// "Linux", so the mobile platforms must be tested before the desktop ones.
func DeviceFromUA(ua string) string {
	s := strings.ToLower(ua)
	switch {
	case strings.Contains(s, "iphone"), strings.Contains(s, "ipad"), strings.Contains(s, "ipod"):
		return DeviceIOS
	case strings.Contains(s, "android"):
		return DeviceAndroid
	case strings.Contains(s, "windows"):
		return DeviceWindows
	case strings.Contains(s, "macintosh"), strings.Contains(s, "mac os x"):
		return DeviceMacOS
	case strings.Contains(s, "linux"):
		return DeviceLinux
	default:
		return DeviceOther
	}
}
