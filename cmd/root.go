package cmd

import (
	"fmt"
	"net"
	"os"
	"time"

	"github.com/buraglio/lldp2map/internal/filter"
	"github.com/buraglio/lldp2map/internal/graph"
	"github.com/buraglio/lldp2map/internal/lldp"
	"github.com/buraglio/lldp2map/internal/render"
	snmpclient "github.com/buraglio/lldp2map/internal/snmp"
	"github.com/spf13/cobra"
)

type queueItem struct {
	host  string
	depth int
}

var (
	community    string
	snmpVersion  string
	username     string
	authProto    string
	authPass     string
	privProto    string
	privPass     string
	secLevel     string
	snmpPort     uint16
	snmpTimeout  int
	snmpRetries  int
	maxHops          int
	showAddrs        bool
	addrFamily       string
	ignorePrefixStrs []string
	outputFile       string
	outputFormat     string
)

var rootCmd = &cobra.Command{
	Use:   "lldp2map <host>",
	Short: "Walk LLDP via SNMP and generate a network topology map",
	Long: `lldp2map discovers network topology by recursively walking LLDP neighbor
tables via SNMP, then renders the result as a PNG or PDF using Graphviz.

Requires Graphviz installed: brew install graphviz (macOS) or apt install graphviz (Linux)

Examples:
  # SNMPv2c
  lldp2map -c public 192.168.1.1

  # SNMPv3 with auth+priv
  lldp2map -v 3 --username admin --auth-proto SHA --auth-pass secret \
            --priv-proto AES --priv-pass secret 192.168.1.1

  # Output PDF, limit discovery depth
  lldp2map -c public -f pdf -o topology.pdf --max-hops 5 10.0.0.1

  # Export to Draw.io
  lldp2map -c public -f drawio 10.0.0.1

  # Export to Excalidraw
  lldp2map -c public -f excalidraw 10.0.0.1`,
	Args: cobra.ExactArgs(1),
	RunE: run,
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	f := rootCmd.Flags()

	// v2c
	f.StringVarP(&community, "community", "c", "public", "SNMPv2c community string")

	// Version
	f.StringVarP(&snmpVersion, "version", "v", "2c", "SNMP version: 2c or 3")

	// v3
	f.StringVar(&username, "username", "", "SNMPv3 username")
	f.StringVar(&authProto, "auth-proto", "SHA", "SNMPv3 auth protocol: MD5, SHA, SHA256, SHA512")
	f.StringVar(&authPass, "auth-pass", "", "SNMPv3 authentication passphrase")
	f.StringVar(&privProto, "priv-proto", "AES", "SNMPv3 priv protocol: DES, AES, AES192, AES256")
	f.StringVar(&privPass, "priv-pass", "", "SNMPv3 privacy passphrase")
	f.StringVar(&secLevel, "sec-level", "authpriv", "SNMPv3 security level: noauth, auth, authpriv")

	// Transport
	f.Uint16Var(&snmpPort, "port", 161, "SNMP UDP port")
	f.IntVar(&snmpTimeout, "timeout", 5, "SNMP timeout in seconds")
	f.IntVar(&snmpRetries, "retries", 2, "SNMP retries per request")

	// Discovery
	f.IntVar(&maxHops, "max-hops", 10, "Maximum BFS depth for recursive discovery")
	f.BoolVar(&showAddrs, "show-addrs", false, "Include interface IPv4/IPv6 addresses in node labels (walks IP-MIB on each device)")
	f.StringVar(&addrFamily, "addr-family", "both", "Address family to display with --show-addrs: ipv4, ipv6, or both")
	f.StringArrayVar(&ignorePrefixStrs, "ignore-prefix", nil, "CIDR prefix to exclude from labels and discovery (repeatable: --ignore-prefix 127.0.0.0/8 --ignore-prefix fd68:1::/48)")

	// Output
	f.StringVarP(&outputFile, "output", "o", "network-map.png", "Output file path")
	f.StringVarP(&outputFormat, "format", "f", "png", "Output format: png, pdf, drawio, excalidraw")
}

func run(_ *cobra.Command, args []string) error {
	seedHost := args[0]

	// Validate --addr-family
	switch addrFamily {
	case "ipv4", "ipv6", "both":
	default:
		return fmt.Errorf("invalid --addr-family %q: must be ipv4, ipv6, or both", addrFamily)
	}

	// Parse --ignore-prefix values once so we can reuse them throughout the run.
	var ignorePrefixes []*net.IPNet
	if len(ignorePrefixStrs) > 0 {
		var err error
		ignorePrefixes, err = filter.ParseIgnorePrefixes(ignorePrefixStrs)
		if err != nil {
			return err
		}
	}

	validFormats := map[string]string{
		"png":        ".png",
		"pdf":        ".pdf",
		"drawio":     ".drawio",
		"excalidraw": ".excalidraw",
	}
	ext, ok := validFormats[outputFormat]
	if !ok {
		return fmt.Errorf("unsupported format %q: use png, pdf, drawio, or excalidraw", outputFormat)
	}
	// Auto-correct the output extension when the user left the default filename.
	if outputFile == "network-map.png" && outputFormat != "png" {
		outputFile = "network-map" + ext
	}

	baseCfg := snmpclient.Config{
		Port:      snmpPort,
		Version:   snmpVersion,
		Community: community,
		Username:  username,
		AuthProto: snmpclient.AuthProto(authProto),
		AuthPass:  authPass,
		PrivProto: snmpclient.PrivProto(privProto),
		PrivPass:  privPass,
		SecLevel:  secLevel,
		Timeout:   time.Duration(snmpTimeout) * time.Second,
		Retries:   snmpRetries,
	}

	topo := graph.New()

	queue := []queueItem{{host: seedHost, depth: 0}}
	visited := map[string]bool{}

	fmt.Printf("Starting LLDP discovery from %s (max-hops=%d)\n\n", seedHost, maxHops)

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		if visited[item.host] {
			continue
		}
		visited[item.host] = true

		indent := fmt.Sprintf("%*s", item.depth*2, "")
		fmt.Printf("%s[hop %d] Querying %s...\n", indent, item.depth, item.host)

		cfg := baseCfg
		cfg.Host = item.host

		client, err := snmpclient.New(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s  Warning: cannot connect to %s: %v\n", indent, item.host, err)
			continue
		}

		info, err := lldp.Walk(client)
		var ifAddrs []string
		if err == nil && showAddrs {
			raw, _ := lldp.WalkIPAddresses(client)
			ifAddrs = filter.Addrs(raw, addrFamily, ignorePrefixes)
		}
		client.Close()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s  Warning: LLDP walk failed on %s: %v\n", indent, item.host, err)
			continue
		}

		localName := info.SysName
		if localName == "" {
			localName = item.host
		}

		topo.AddNode(localName, item.host)
		if len(ifAddrs) > 0 {
			topo.SetAddrs(localName, ifAddrs)
		}

		if len(info.Neighbors) == 0 {
			fmt.Printf("%s  No LLDP neighbors found.\n", indent)
			continue
		}

		fmt.Printf("%s  Found %d neighbor(s) on %s:\n", indent, len(info.Neighbors), localName)

		for _, neighbor := range info.Neighbors {
			if neighbor.RemoteSys == "" {
				continue
			}

			topo.AddNode(neighbor.RemoteSys, "")
			topo.AddEdge(localName, neighbor.RemoteSys, neighbor.LocalPort, neighbor.RemotePort)

			fmt.Printf("%s    %-30s  %s -> %s\n",
				indent, neighbor.RemoteSys, neighbor.LocalPort, neighbor.RemotePort)

			if item.depth >= maxHops {
				continue
			}

			for _, ip := range filter.IPs(neighbor.MgmtAddrs, addrFamily, ignorePrefixes) {
				ipStr := ip.String()
				if !visited[ipStr] && !inQueue(queue, ipStr) {
					queue = append(queue, queueItem{host: ipStr, depth: item.depth + 1})
					topo.AddNode(neighbor.RemoteSys, ipStr)
				}
			}
		}
	}

	nodeCount := len(topo.Nodes)
	edgeCount := len(topo.Edges)

	if nodeCount == 0 {
		return fmt.Errorf("no LLDP data discovered from %s\n"+
			"Check SNMP credentials and that LLDP is enabled on the device", seedHost)
	}

	fmt.Printf("\nDiscovered %d node(s), %d link(s)\n", nodeCount, edgeCount)
	fmt.Printf("Rendering %s -> %s...\n", outputFormat, outputFile)

	if err := render.Render(topo, outputFile, outputFormat); err != nil {
		return err
	}

	fmt.Printf("Done. Output saved to: %s\n", outputFile)
	return nil
}

func inQueue(queue []queueItem, host string) bool {
	for _, item := range queue {
		if item.host == host {
			return true
		}
	}
	return false
}
