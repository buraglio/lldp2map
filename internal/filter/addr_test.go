package filter

import (
	"net"
	"testing"
)

// helper: parse a CIDR and return only the *net.IPNet, panicking on error.
func mustParseCIDR(s string) *net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	return n
}

// helper: parse an IP, panicking on failure.
func mustParseIP(s string) net.IP {
	ip := net.ParseIP(s)
	if ip == nil {
		panic("invalid IP: " + s)
	}
	return ip
}

// ── ParseIgnorePrefixes ──────────────────────────────────────────────────────

func TestParseIgnorePrefixes_Valid(t *testing.T) {
	inputs := []string{"10.0.0.0/8", "172.16.0.0/12", "fd00::/8"}
	nets, err := ParseIgnorePrefixes(inputs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nets) != len(inputs) {
		t.Fatalf("got %d nets, want %d", len(nets), len(inputs))
	}
}

func TestParseIgnorePrefixes_Empty(t *testing.T) {
	nets, err := ParseIgnorePrefixes(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nets) != 0 {
		t.Fatalf("expected empty result, got %d nets", len(nets))
	}
}

func TestParseIgnorePrefixes_Invalid(t *testing.T) {
	_, err := ParseIgnorePrefixes([]string{"10.0.0.0/8", "not-a-cidr"})
	if err == nil {
		t.Fatal("expected error for invalid CIDR, got nil")
	}
}

func TestParseIgnorePrefixes_HostBitsSet(t *testing.T) {
	// net.ParseCIDR accepts host-bits-set notation and masks them; should not error.
	_, err := ParseIgnorePrefixes([]string{"192.168.1.1/24"})
	if err != nil {
		t.Fatalf("unexpected error for host-bits-set CIDR: %v", err)
	}
}

// ── Addrs ────────────────────────────────────────────────────────────────────

func TestAddrs_Empty(t *testing.T) {
	got := Addrs(nil, "both", nil)
	if len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
}

func TestAddrs_FamilyBoth(t *testing.T) {
	in := []string{"10.0.0.1", "2001:db8::1"}
	got := Addrs(in, "both", nil)
	if len(got) != 2 {
		t.Fatalf("expected 2, got %v", got)
	}
}

func TestAddrs_FamilyDefault(t *testing.T) {
	// Empty string should behave the same as "both".
	in := []string{"10.0.0.1", "2001:db8::1"}
	got := Addrs(in, "", nil)
	if len(got) != 2 {
		t.Fatalf("expected 2, got %v", got)
	}
}

func TestAddrs_FamilyIPv4Only(t *testing.T) {
	in := []string{"10.0.0.1", "2001:db8::1", "192.168.1.1"}
	got := Addrs(in, "ipv4", nil)
	if len(got) != 2 {
		t.Fatalf("expected 2 IPv4 addresses, got %v", got)
	}
	for _, s := range got {
		if net.ParseIP(s).To4() == nil {
			t.Errorf("non-IPv4 address leaked through: %s", s)
		}
	}
}

func TestAddrs_FamilyIPv6Only(t *testing.T) {
	in := []string{"10.0.0.1", "2001:db8::1", "fd00::1"}
	got := Addrs(in, "ipv6", nil)
	if len(got) != 2 {
		t.Fatalf("expected 2 IPv6 addresses, got %v", got)
	}
	for _, s := range got {
		if net.ParseIP(s).To4() != nil {
			t.Errorf("IPv4 address leaked through: %s", s)
		}
	}
}

func TestAddrs_IgnorePrefix(t *testing.T) {
	ignore := []*net.IPNet{mustParseCIDR("10.0.0.0/8")}
	in := []string{"10.0.0.1", "10.255.255.255", "192.168.1.1"}
	got := Addrs(in, "both", ignore)
	if len(got) != 1 || got[0] != "192.168.1.1" {
		t.Fatalf("expected [192.168.1.1], got %v", got)
	}
}

func TestAddrs_IgnorePrefixIPv6(t *testing.T) {
	ignore := []*net.IPNet{mustParseCIDR("fd00::/8")}
	in := []string{"fd00::1", "fd68:1::1", "2001:db8::1"}
	got := Addrs(in, "both", ignore)
	if len(got) != 1 || got[0] != "2001:db8::1" {
		t.Fatalf("expected [2001:db8::1], got %v", got)
	}
}

func TestAddrs_MultipleIgnorePrefixes(t *testing.T) {
	ignore := []*net.IPNet{
		mustParseCIDR("127.0.0.0/8"),
		mustParseCIDR("192.168.0.0/16"),
		mustParseCIDR("fd00::/8"),
	}
	in := []string{"127.0.0.1", "192.168.1.1", "fd68:1::1", "10.0.0.1", "2001:db8::1"}
	got := Addrs(in, "both", ignore)
	if len(got) != 2 {
		t.Fatalf("expected 2 addresses, got %v", got)
	}
}

func TestAddrs_FamilyAndIgnoreCombined(t *testing.T) {
	// IPv4 only, ignoring 10.0.0.0/8
	ignore := []*net.IPNet{mustParseCIDR("10.0.0.0/8")}
	in := []string{"10.0.0.1", "192.168.1.1", "2001:db8::1"}
	got := Addrs(in, "ipv4", ignore)
	if len(got) != 1 || got[0] != "192.168.1.1" {
		t.Fatalf("expected [192.168.1.1], got %v", got)
	}
}

func TestAddrs_InvalidStringDropped(t *testing.T) {
	in := []string{"not-an-ip", "10.0.0.1"}
	got := Addrs(in, "both", nil)
	if len(got) != 1 || got[0] != "10.0.0.1" {
		t.Fatalf("expected [10.0.0.1], got %v", got)
	}
}

func TestAddrs_AllFiltered(t *testing.T) {
	ignore := []*net.IPNet{mustParseCIDR("0.0.0.0/0")}
	in := []string{"10.0.0.1", "192.168.1.1"}
	got := Addrs(in, "both", ignore)
	if len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
}

// ── IPs ──────────────────────────────────────────────────────────────────────

func TestIPs_Empty(t *testing.T) {
	got := IPs(nil, "both", nil)
	if len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
}

func TestIPs_FamilyBoth(t *testing.T) {
	in := []net.IP{mustParseIP("10.0.0.1"), mustParseIP("2001:db8::1")}
	got := IPs(in, "both", nil)
	if len(got) != 2 {
		t.Fatalf("expected 2, got %v", got)
	}
}

func TestIPs_FamilyDefault(t *testing.T) {
	in := []net.IP{mustParseIP("10.0.0.1"), mustParseIP("2001:db8::1")}
	got := IPs(in, "", nil)
	if len(got) != 2 {
		t.Fatalf("expected 2, got %v", got)
	}
}

func TestIPs_FamilyIPv4Only(t *testing.T) {
	in := []net.IP{mustParseIP("10.0.0.1"), mustParseIP("2001:db8::1"), mustParseIP("172.16.0.1")}
	got := IPs(in, "ipv4", nil)
	if len(got) != 2 {
		t.Fatalf("expected 2, got %v", got)
	}
	for _, ip := range got {
		if ip.To4() == nil {
			t.Errorf("non-IPv4 leaked through: %v", ip)
		}
	}
}

func TestIPs_FamilyIPv6Only(t *testing.T) {
	in := []net.IP{mustParseIP("10.0.0.1"), mustParseIP("2001:db8::1"), mustParseIP("fd00::1")}
	got := IPs(in, "ipv6", nil)
	if len(got) != 2 {
		t.Fatalf("expected 2, got %v", got)
	}
	for _, ip := range got {
		if ip.To4() != nil {
			t.Errorf("IPv4 leaked through: %v", ip)
		}
	}
}

func TestIPs_IgnorePrefix(t *testing.T) {
	ignore := []*net.IPNet{mustParseCIDR("192.168.0.0/16")}
	in := []net.IP{mustParseIP("192.168.1.1"), mustParseIP("10.0.0.1")}
	got := IPs(in, "both", ignore)
	if len(got) != 1 || got[0].String() != "10.0.0.1" {
		t.Fatalf("expected [10.0.0.1], got %v", got)
	}
}

func TestIPs_IgnorePrefixIPv6(t *testing.T) {
	ignore := []*net.IPNet{mustParseCIDR("fd68:1::/48")}
	in := []net.IP{mustParseIP("fd68:1::1"), mustParseIP("2001:db8::1")}
	got := IPs(in, "both", ignore)
	if len(got) != 1 || got[0].String() != "2001:db8::1" {
		t.Fatalf("expected [2001:db8::1], got %v", got)
	}
}

func TestIPs_MultipleIgnorePrefixes(t *testing.T) {
	ignore := []*net.IPNet{
		mustParseCIDR("10.0.0.0/8"),
		mustParseCIDR("fd00::/8"),
	}
	in := []net.IP{
		mustParseIP("10.0.0.1"),
		mustParseIP("fd68:1::1"),
		mustParseIP("192.168.1.1"),
		mustParseIP("2001:db8::1"),
	}
	got := IPs(in, "both", ignore)
	if len(got) != 2 {
		t.Fatalf("expected 2, got %v", got)
	}
}

func TestIPs_FamilyAndIgnoreCombined(t *testing.T) {
	// IPv6 only, ignoring fd00::/8
	ignore := []*net.IPNet{mustParseCIDR("fd00::/8")}
	in := []net.IP{
		mustParseIP("10.0.0.1"),
		mustParseIP("fd00::1"),
		mustParseIP("2001:db8::1"),
	}
	got := IPs(in, "ipv6", ignore)
	if len(got) != 1 || got[0].String() != "2001:db8::1" {
		t.Fatalf("expected [2001:db8::1], got %v", got)
	}
}

func TestIPs_AllFiltered(t *testing.T) {
	ignore := []*net.IPNet{mustParseCIDR("0.0.0.0/0"), mustParseCIDR("::/0")}
	in := []net.IP{mustParseIP("10.0.0.1"), mustParseIP("2001:db8::1")}
	got := IPs(in, "both", ignore)
	if len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
}

func TestIPs_InputNotMutated(t *testing.T) {
	// Verify that the original slice is not modified by filtering.
	in := []net.IP{mustParseIP("10.0.0.1"), mustParseIP("2001:db8::1")}
	orig := make([]net.IP, len(in))
	copy(orig, in)

	IPs(in, "ipv4", nil)

	if len(in) != len(orig) {
		t.Fatalf("original slice length changed: got %d, want %d", len(in), len(orig))
	}
	for i := range in {
		if !in[i].Equal(orig[i]) {
			t.Errorf("original slice[%d] mutated: got %v, want %v", i, in[i], orig[i])
		}
	}
}
