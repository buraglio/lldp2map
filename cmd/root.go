package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/buraglio/lldp2map/internal/discover"
	"github.com/spf13/cobra"
)

var (
	community        string
	snmpVersion      string
	username         string
	authProto        string
	authPass         string
	privProto        string
	privPass         string
	secLevel         string
	snmpPort         uint16
	snmpTimeout      int
	snmpRetries      int
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
	f.StringVar(&addrFamily, "addr-family", "both", "Address family to display/follow when --show-addrs is set: ipv4, ipv6, or both")
	f.StringArrayVar(&ignorePrefixStrs, "ignore-prefix", nil, "CIDR prefix to exclude from labels and next-hop discovery (repeatable)")

	// Output
	f.StringVarP(&outputFile, "output", "o", "network-map.png", "Output file path")
	f.StringVarP(&outputFormat, "format", "f", "png", "Output format: png, pdf, drawio, excalidraw")
}

func run(_ *cobra.Command, args []string) error {
	seedHost := args[0]

	// Validate --addr-family early so the user gets a clear error before
	// any SNMP connections are attempted.
	switch addrFamily {
	case "ipv4", "ipv6", "both":
	default:
		return fmt.Errorf("invalid --addr-family %q: must be ipv4, ipv6, or both", addrFamily)
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

	cfg := discover.Config{
		SeedHost:         seedHost,
		Community:        community,
		Version:          snmpVersion,
		Username:         username,
		AuthProto:        authProto,
		AuthPass:         authPass,
		PrivProto:        privProto,
		PrivPass:         privPass,
		SecLevel:         secLevel,
		Port:             snmpPort,
		Timeout:          snmpTimeout,
		Retries:          snmpRetries,
		MaxHops:          maxHops,
		ShowAddrs:        showAddrs,
		AddrFamily:       addrFamily,
		IgnorePrefixStrs: ignorePrefixStrs,
		OutputFile:       outputFile,
		OutputFormat:     outputFormat,
	}

	_, err := discover.Run(context.Background(), cfg, func(msg string) {
		fmt.Println(msg)
	})
	return err
}
