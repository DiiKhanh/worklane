package http

import (
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/idna"
)

// Non-public special-purpose ranges from the IANA IPv4 and IPv6 registries:
// https://www.iana.org/assignments/iana-ipv4-special-registry
// https://www.iana.org/assignments/iana-ipv6-special-registry
var nonPublic = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"), netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"), netip.MustParsePrefix("::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"), netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("100:0:0:1::/64"), netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("3fff::/20"), netip.MustParsePrefix("5f00::/16"),
	netip.MustParsePrefix("fec0::/10"),
}

var publicExceptions = []netip.Prefix{
	netip.MustParsePrefix("192.0.0.9/32"), netip.MustParsePrefix("192.0.0.10/32"),
	netip.MustParsePrefix("2001:1::1/128"), netip.MustParsePrefix("2001:1::2/128"),
	netip.MustParsePrefix("2001:1::3/128"), netip.MustParsePrefix("2001:3::/32"),
	netip.MustParsePrefix("2001:4:112::/48"), netip.MustParsePrefix("2001:20::/28"),
	netip.MustParsePrefix("2001:30::/28"),
}

// IPv6 global unicast allocations currently come from 2000::/3. The
// well-known NAT64 translation prefix is separately globally reachable.
var (
	globalIPv6 = netip.MustParsePrefix("2000::/3")
	nat64IPv6  = netip.MustParsePrefix("64:ff9b::/96")
)

func validDestination(raw, shortHost string) bool {
	if len(raw) == 0 || len(raw) > MaxDestinationBytes || !utf8.ValidString(raw) {
		return false
	}
	for _, r := range raw {
		if unicode.IsControl(r) || unicode.IsSpace(r) || r == '\\' {
			return false
		}
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Opaque != "" || u.User != nil || u.Hostname() == "" {
		return false
	}
	if strings.HasSuffix(u.Host, ":") {
		return false
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return false
		}
	}
	host, err := normalizedHost(u.Hostname())
	if err != nil || host == shortHost || host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return false
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		return publicIP(ip)
	}
	if strings.ContainsAny(host, ":%[]") || strings.HasPrefix(u.Host, "[") {
		return false
	}
	// Browsers interpret numeric and legacy hex/octal hosts as IP addresses.
	// Reject those ambiguous spellings instead of treating them as DNS names.
	labels := strings.Split(host, ".")
	last := labels[len(labels)-1]
	if _, err := strconv.ParseUint(last, 10, 64); err == nil {
		return false
	}
	if strings.HasPrefix(last, "0x") {
		return false
	}
	return host != ""
}

func normalizedHost(host string) (string, error) {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if ip, err := netip.ParseAddr(host); err == nil {
		return ip.Unmap().String(), nil
	}
	ascii, err := idna.Lookup.ToASCII(host)
	return strings.TrimSuffix(strings.ToLower(ascii), "."), err
}

func publicIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if ip.Zone() != "" || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	if ip.Is6() && !globalIPv6.Contains(ip) && !nat64IPv6.Contains(ip) {
		return false
	}
	for _, prefix := range publicExceptions {
		if prefix.Contains(ip) {
			return true
		}
	}
	for _, prefix := range nonPublic {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}
