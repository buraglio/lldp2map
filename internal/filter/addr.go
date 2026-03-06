package filter

import (
	"fmt"
	"net"
)

// ParseIgnorePrefixes parses a slice of CIDR strings into []*net.IPNet.
// Returns an error if any string is not a valid CIDR prefix.
func ParseIgnorePrefixes(prefixes []string) ([]*net.IPNet, error) {
	nets := make([]*net.IPNet, 0, len(prefixes))
	for _, p := range prefixes {
		_, ipNet, err := net.ParseCIDR(p)
		if err != nil {
			return nil, fmt.Errorf("invalid prefix %q: %w", p, err)
		}
		nets = append(nets, ipNet)
	}
	return nets, nil
}

// isIPv4 reports whether ip is an IPv4 address.
func isIPv4(ip net.IP) bool {
	return ip.To4() != nil
}

// matchesFamily reports whether ip matches the requested address family.
// family "ipv4" accepts only IPv4, "ipv6" accepts only IPv6, anything else
// (including "" and "both") accepts all addresses.
func matchesFamily(ip net.IP, family string) bool {
	switch family {
	case "ipv4":
		return isIPv4(ip)
	case "ipv6":
		return !isIPv4(ip)
	default:
		return true
	}
}

// inIgnored reports whether ip is covered by any of the ignore prefixes.
func inIgnored(ip net.IP, ignore []*net.IPNet) bool {
	for _, n := range ignore {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// Addrs filters a slice of IP address strings by address family and ignore
// prefixes. Addresses that do not parse, do not match the requested family,
// or are covered by an ignore prefix are dropped.
// family: "ipv4", "ipv6", or "" / "both" (no family filtering).
func Addrs(addrs []string, family string, ignore []*net.IPNet) []string {
	if len(addrs) == 0 {
		return addrs
	}
	out := make([]string, 0, len(addrs))
	for _, s := range addrs {
		ip := net.ParseIP(s)
		if ip == nil {
			continue
		}
		if !matchesFamily(ip, family) || inIgnored(ip, ignore) {
			continue
		}
		out = append(out, s)
	}
	return out
}

// IPs filters a slice of net.IP by address family and ignore prefixes.
// IPs that do not match the requested family or are covered by an ignore
// prefix are dropped.
func IPs(ips []net.IP, family string, ignore []*net.IPNet) []net.IP {
	if len(ips) == 0 {
		return ips
	}
	out := make([]net.IP, 0, len(ips))
	for _, ip := range ips {
		if !matchesFamily(ip, family) || inIgnored(ip, ignore) {
			continue
		}
		out = append(out, ip)
	}
	return out
}
