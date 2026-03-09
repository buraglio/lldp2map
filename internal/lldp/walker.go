package lldp

import (
	"fmt"
	"net"
	"strings"

	snmpclient "github.com/buraglio/lldp2map/internal/snmp"
	"github.com/gosnmp/gosnmp"
)

// LLDP-MIB OIDs (IEEE 802.1AB)
const (
	// Local system
	oidLocSysName  = "1.0.8802.1.1.2.1.3.3.0"
	oidLocPortDesc = "1.0.8802.1.1.2.1.3.7.1.4"

	// Remote neighbor table (index: timeMark.portNum.remIndex)
	oidRemPortId   = "1.0.8802.1.1.2.1.4.1.1.7"
	oidRemPortDesc = "1.0.8802.1.1.2.1.4.1.1.8"

	// IP-MIB (RFC 4293) — modern unified IPv4+IPv6 address table.
	// Index: InetAddressType.InetAddressLength.InetAddress[bytes]
	// addrType 1=IPv4 (4 bytes), 2=IPv6 (16 bytes)
	oidIPAddressIfIndex = "1.3.6.1.2.1.4.34.1.3"

	// IP-MIB (RFC 1213) — legacy IPv4-only address table, fallback.
	// Index: the IPv4 address itself (4 octets).
	oidIPAdEntAddr = "1.3.6.1.2.1.4.20.1.1"
	oidRemSysName  = "1.0.8802.1.1.2.1.4.1.1.9"

	// Remote management address table
	// Index: timeMark.portNum.remIndex.addrSubtype.addrLen.addr[bytes]
	oidRemManAddrIfId = "1.0.8802.1.1.2.1.4.2.1.3"

	// Remote chassis ID — used as fallback when no management address is advertised.
	// Index: timeMark.portNum.remIndex
	// oidRemChassisIdSubtype: 4=macAddress, 5=networkAddress
	oidRemChassisIdSubtype = "1.0.8802.1.1.2.1.4.1.1.4"
	oidRemChassisId        = "1.0.8802.1.1.2.1.4.1.1.5"

	// ARP table — IPv4 MAC→IP mapping on the queried device.
	// Index: ifIndex.a.b.c.d  Value: MAC (6 bytes)
	oidARPPhysAddr = "1.3.6.1.2.1.4.22.1.2"

	// ipNetToPhysicalTable (RFC 4293) — unified IPv4+IPv6 neighbor table.
	// Index: ifIndex.addrType.addrLen.addr[bytes]  Value: MAC (6 bytes)
	// addrType 1=IPv4 (4 bytes), 2=IPv6 (16 bytes)
	oidIPNetToPhysicalPhysAddr = "1.3.6.1.2.1.4.35.1.4"
)

// Neighbor represents a single LLDP-discovered neighbor.
type Neighbor struct {
	LocalPort  string
	RemoteSys  string
	RemotePort string
	MgmtAddrs  []net.IP
}

// LocalInfo holds the local device name and all discovered neighbors.
type LocalInfo struct {
	SysName   string
	Neighbors []Neighbor
}

// remKey uniquely identifies a remote neighbor entry by local port and remote index.
type remKey struct{ portNum, remIndex string }

// Walk queries LLDP MIB tables on the device and returns discovered neighbors.
func Walk(client *snmpclient.Client) (*LocalInfo, error) {
	info := &LocalInfo{}

	// Local system name
	if pdu, err := client.Get(oidLocSysName); err == nil {
		info.SysName = pduToString(pdu)
	}

	// Local port descriptions: index = portNum
	localPorts := map[string]string{}
	if pdus, err := client.Walk(oidLocPortDesc); err == nil {
		for _, pdu := range pdus {
			portNum := suffixAfter(pdu.Name, oidLocPortDesc)
			if portNum != "" {
				localPorts[portNum] = pduToString(&pdu)
			}
		}
	}

	// Remote sys names: index = timeMark.portNum.remIndex
	sysNames := map[remKey]string{}
	pdus, err := client.Walk(oidRemSysName)
	if err != nil {
		// Return whatever partial info we have so the caller can still register
		// this node in the topology even if the full LLDP walk fails.
		return info, fmt.Errorf("walk lldpRemSysName: %w", err)
	}
	for _, pdu := range pdus {
		if k, ok := parseRemKey(pdu.Name, oidRemSysName); ok {
			sysNames[k] = pduToString(&pdu)
		}
	}

	// Remote port descriptions
	remPortDescs := map[remKey]string{}
	if pdus, err := client.Walk(oidRemPortDesc); err == nil {
		for _, pdu := range pdus {
			if k, ok := parseRemKey(pdu.Name, oidRemPortDesc); ok {
				remPortDescs[k] = pduToString(&pdu)
			}
		}
	}

	// Remote port IDs (fallback when description is empty)
	remPortIds := map[remKey]string{}
	if pdus, err := client.Walk(oidRemPortId); err == nil {
		for _, pdu := range pdus {
			if k, ok := parseRemKey(pdu.Name, oidRemPortId); ok {
				remPortIds[k] = pduToString(&pdu)
			}
		}
	}

	// Remote management addresses
	mgmtAddrs := map[remKey][]net.IP{}
	if pdus, err := client.Walk(oidRemManAddrIfId); err == nil {
		for _, pdu := range pdus {
			k, ip, ok := parseMgmtAddr(pdu.Name)
			if ok && ip != nil {
				mgmtAddrs[k] = append(mgmtAddrs[k], ip)
			}
		}
	}

	// Chassis ID subtypes: 4=macAddress, 5=networkAddress
	chassisSubtypes := map[remKey]int{}
	if pdus, err := client.Walk(oidRemChassisIdSubtype); err == nil {
		for _, pdu := range pdus {
			if k, ok := parseRemKey(pdu.Name, oidRemChassisIdSubtype); ok {
				switch v := pdu.Value.(type) {
				case int:
					chassisSubtypes[k] = v
				case uint:
					chassisSubtypes[k] = int(v)
				}
			}
		}
	}

	// Chassis IDs: extract IP for subtype 5 (networkAddress) and collect MACs for subtype 4.
	chassisIPs := map[remKey]net.IP{}
	chassisMACs := map[remKey][6]byte{}
	if pdus, err := client.Walk(oidRemChassisId); err == nil {
		for _, pdu := range pdus {
			if k, ok := parseRemKey(pdu.Name, oidRemChassisId); ok {
				b, isByte := pdu.Value.([]byte)
				if !isByte {
					continue
				}
				switch chassisSubtypes[k] {
				case 5: // networkAddress — IP encoded directly
					if ip := parseNetworkAddress(b); ip != nil {
						chassisIPs[k] = ip
					}
				case 4: // macAddress — resolve via ARP table
					if len(b) == 6 {
						var mac [6]byte
						copy(mac[:], b)
						chassisMACs[k] = mac
					}
				}
			}
		}
	}

	// For MAC-identified chassis IDs, resolve to an IP address.
	// Prefer the RFC 4293 unified IPv4+IPv6 neighbor table; fall back to the
	// legacy IPv4-only ARP table for devices that don't implement RFC 4293.
	if len(chassisMACs) > 0 {
		neighborMap := walkIPNetToPhysical(client)
		if len(neighborMap) == 0 {
			neighborMap = walkARP(client)
		}
		for k, mac := range chassisMACs {
			if _, already := chassisIPs[k]; !already {
				if ip, found := neighborMap[mac]; found {
					chassisIPs[k] = ip
				}
			}
		}
	}

	// Assemble neighbor list
	for k, sysName := range sysNames {
		if sysName == "" {
			continue
		}

		localPort := localPorts[k.portNum]
		if localPort == "" {
			localPort = "port-" + k.portNum
		}

		remotePort := remPortDescs[k]
		if remotePort == "" {
			remotePort = remPortIds[k]
		}

		addrs := mgmtAddrs[k]
		// Fall back to chassis IP when no explicit management address is advertised.
		if len(addrs) == 0 {
			if ip := chassisIPs[k]; ip != nil {
				addrs = []net.IP{ip}
			}
		}

		info.Neighbors = append(info.Neighbors, Neighbor{
			LocalPort:  localPort,
			RemoteSys:  sysName,
			RemotePort: remotePort,
			MgmtAddrs:  addrs,
		})
	}

	return info, nil
}

// suffixAfter strips the base OID prefix (with leading dot) and returns the suffix.
// Returns "" if the OID doesn't start with the expected prefix.
func suffixAfter(oidName, base string) string {
	prefix := "." + base + "."
	if !strings.HasPrefix(oidName, prefix) {
		// Try without leading dot on base
		prefix = base + "."
		if !strings.HasPrefix(oidName, prefix) {
			return ""
		}
	}
	return strings.TrimPrefix(oidName, prefix)
}

// parseRemKey extracts a remKey from an OID with index timeMark.portNum.remIndex.
func parseRemKey(oidName, base string) (remKey, bool) {
	suffix := suffixAfter(oidName, base)
	if suffix == "" {
		return remKey{}, false
	}
	parts := strings.SplitN(suffix, ".", 3)
	if len(parts) < 3 {
		return remKey{}, false
	}
	return remKey{portNum: parts[1], remIndex: parts[2]}, true
}

// parseMgmtAddr parses a management address OID.
// Index: timeMark.portNum.remIndex.addrSubtype.addrLen.addr[bytes]
// addrSubtype 1 = IPv4 (4 bytes), 2 = IPv6 (16 bytes)
func parseMgmtAddr(oidName string) (remKey, net.IP, bool) {
	suffix := suffixAfter(oidName, oidRemManAddrIfId)
	if suffix == "" {
		return remKey{}, nil, false
	}

	parts := strings.Split(suffix, ".")
	// Minimum: timeMark(1) portNum(1) remIndex(1) addrSubtype(1) addrLen(1) addr(>=4)
	if len(parts) < 9 {
		return remKey{}, nil, false
	}

	k := remKey{portNum: parts[1], remIndex: parts[2]}
	addrSubtype := parts[3]
	// parts[4] = addrLen (we trust the subtype to determine length)

	switch addrSubtype {
	case "1": // IPv4
		if len(parts) < 9 {
			return k, nil, false
		}
		ipStr := strings.Join(parts[5:9], ".")
		ip := net.ParseIP(ipStr)
		return k, ip, ip != nil
	case "2": // IPv6
		if len(parts) < 21 {
			return k, nil, false
		}
		b := make([]byte, 16)
		for i := 0; i < 16; i++ {
			var v int
			fmt.Sscanf(parts[5+i], "%d", &v)
			b[i] = byte(v)
		}
		ip := net.IP(b)
		return k, ip, true
	}

	return k, nil, false
}

// parseNetworkAddress decodes a networkAddress chassis ID byte slice.
// Format: [IANA address family (1 byte)] [address bytes]
// Family 1 = IPv4 (4 bytes), Family 2 = IPv6 (16 bytes).
func parseNetworkAddress(b []byte) net.IP {
	if len(b) == 5 && b[0] == 1 {
		return net.IP(b[1:5]).To4()
	}
	if len(b) == 17 && b[0] == 2 {
		return net.IP(b[1:17])
	}
	return nil
}

// walkARP walks the IPv4 ARP table (ipNetToMediaPhysAddress) on the device and
// returns a map from MAC address to IPv4 address.
// OID index: ifIndex.a.b.c.d  Value: 6-byte MAC
func walkARP(client *snmpclient.Client) map[[6]byte]net.IP {
	result := map[[6]byte]net.IP{}
	pdus, err := client.Walk(oidARPPhysAddr)
	if err != nil {
		return result
	}
	for _, pdu := range pdus {
		suffix := suffixAfter(pdu.Name, oidARPPhysAddr)
		if suffix == "" {
			continue
		}
		parts := strings.Split(suffix, ".")
		// Index is ifIndex + 4 IP octets — need at least 5 parts.
		if len(parts) < 5 {
			continue
		}
		ipStr := strings.Join(parts[len(parts)-4:], ".")
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}
		ip = ip.To4()
		if ip == nil {
			continue
		}
		b, ok := pdu.Value.([]byte)
		if !ok || len(b) != 6 {
			continue
		}
		var mac [6]byte
		copy(mac[:], b)
		result[mac] = ip
	}
	return result
}

// walkIPNetToPhysical walks the RFC 4293 ipNetToPhysicalTable, which contains
// both IPv4 (ARP) and IPv6 (NDP) neighbor entries. Returns a MAC→IP map
// preferring a global-unicast IPv6 address over IPv4 when both are present
// for the same MAC. Loopback and link-local addresses are excluded.
// Index format: ifIndex.addrType.addrLen.addr[bytes]  Value: MAC (6 bytes)
func walkIPNetToPhysical(client *snmpclient.Client) map[[6]byte]net.IP {
	result := map[[6]byte]net.IP{}
	pdus, err := client.Walk(oidIPNetToPhysicalPhysAddr)
	if err != nil {
		return result
	}
	for _, pdu := range pdus {
		suffix := suffixAfter(pdu.Name, oidIPNetToPhysicalPhysAddr)
		if suffix == "" {
			continue
		}
		parts := strings.Split(suffix, ".")
		// Minimum: ifIndex(1) addrType(1) addrLen(1) addr(>=4) = 7 parts for IPv4.
		if len(parts) < 7 {
			continue
		}
		addrType := parts[1] // 1=IPv4, 2=IPv6

		var ip net.IP
		switch addrType {
		case "1": // IPv4: addrLen always 4, addr starts at parts[3]
			if len(parts) >= 7 {
				ip = net.ParseIP(strings.Join(parts[3:7], "."))
			}
		case "2": // IPv6: addrLen always 16, addr starts at parts[3]
			if len(parts) >= 19 {
				b := make([]byte, 16)
				for i := 0; i < 16; i++ {
					var v int
					fmt.Sscanf(parts[3+i], "%d", &v)
					b[i] = byte(v)
				}
				ip = net.IP(b)
			}
		}

		if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			continue
		}

		mac, ok := pdu.Value.([]byte)
		if !ok || len(mac) != 6 {
			continue
		}
		var key [6]byte
		copy(key[:], mac)

		// Prefer global-unicast IPv6 over IPv4 when multiple entries share a MAC.
		existing, has := result[key]
		if !has || (existing.To4() != nil && ip.To4() == nil) {
			result[key] = ip
		}
	}
	return result
}

// WalkIPAddresses returns all non-loopback, non-link-local unicast interface
// addresses on the device. It queries the modern ipAddressTable (RFC 4293)
// which covers both IPv4 and IPv6, falling back to the legacy IPv4-only
// ipAddrTable (RFC 1213) if the modern table is unavailable or empty.
func WalkIPAddresses(client *snmpclient.Client) ([]string, error) {
	addrs, err := walkIPAddressTable(client)
	if err != nil || len(addrs) == 0 {
		return walkIPv4AddrTable(client)
	}
	return addrs, nil
}

// walkIPAddressTable parses the RFC 4293 ipAddressTable.
// OID index format: addrType.addrLen.addr[bytes]
func walkIPAddressTable(client *snmpclient.Client) ([]string, error) {
	pdus, err := client.Walk(oidIPAddressIfIndex)
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	var addrs []string

	for _, pdu := range pdus {
		suffix := suffixAfter(pdu.Name, oidIPAddressIfIndex)
		if suffix == "" {
			continue
		}
		parts := strings.Split(suffix, ".")
		if len(parts) < 3 {
			continue
		}

		addrType := parts[0] // 1=IPv4, 2=IPv6
		// parts[1] = addrLen

		var ip net.IP
		switch addrType {
		case "1": // IPv4: type(1) len(1) addr(4) = min 6 parts
			if len(parts) >= 6 {
				ip = net.ParseIP(strings.Join(parts[2:6], "."))
			}
		case "2": // IPv6: type(1) len(1) addr(16) = min 18 parts
			if len(parts) >= 18 {
				b := make([]byte, 16)
				for i := 0; i < 16; i++ {
					var v int
					fmt.Sscanf(parts[2+i], "%d", &v)
					b[i] = byte(v)
				}
				ip = net.IP(b)
			}
		}

		if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			continue
		}
		s := ip.String()
		if !seen[s] {
			seen[s] = true
			addrs = append(addrs, s)
		}
	}

	return addrs, nil
}

// walkIPv4AddrTable parses the legacy RFC 1213 ipAddrTable (IPv4 only).
// The OID index is the IPv4 address itself (4 octets).
func walkIPv4AddrTable(client *snmpclient.Client) ([]string, error) {
	pdus, err := client.Walk(oidIPAdEntAddr)
	if err != nil {
		return nil, err
	}

	var addrs []string
	for _, pdu := range pdus {
		suffix := suffixAfter(pdu.Name, oidIPAdEntAddr)
		if suffix == "" {
			continue
		}
		ip := net.ParseIP(suffix)
		if ip == nil || ip.IsLoopback() {
			continue
		}
		addrs = append(addrs, ip.String())
	}
	return addrs, nil
}

// pduToString converts a PDU value to a printable string.
// Control characters (including embedded newlines and NUL bytes) are stripped
// so they cannot corrupt node labels in the rendered diagram.
func pduToString(pdu *gosnmp.SnmpPDU) string {
	switch v := pdu.Value.(type) {
	case string:
		return cleanString(v)
	case []byte:
		// Strip non-printable bytes; if anything useful remains, return it as text.
		cleaned := cleanString(string(v))
		if cleaned != "" {
			return cleaned
		}
		return fmt.Sprintf("%x", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// cleanString removes control characters and trims surrounding whitespace.
func cleanString(s string) string {
	cleaned := strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1 // drop control chars and DEL
		}
		return r
	}, s)
	return strings.TrimSpace(cleaned)
}
