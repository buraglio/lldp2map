package discover

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/buraglio/lldp2map/internal/filter"
	"github.com/buraglio/lldp2map/internal/graph"
	"github.com/buraglio/lldp2map/internal/lldp"
	"github.com/buraglio/lldp2map/internal/render"
	snmpclient "github.com/buraglio/lldp2map/internal/snmp"
)

// Config holds all parameters for a single discovery run.
type Config struct {
	SeedHost         string
	Community        string
	Version          string // "2c" or "3"
	Username         string
	AuthProto        string
	AuthPass         string
	PrivProto        string
	PrivPass         string
	SecLevel         string
	Port             uint16
	Timeout          int // seconds
	Retries          int
	MaxHops          int
	ShowAddrs        bool
	AddrFamily       string   // "ipv4", "ipv6", or "both" / ""
	IgnorePrefixStrs []string // CIDR strings excluded from labels and next-hop discovery
	OutputFile       string
	OutputFormat     string
}

type queueItem struct {
	host  string
	depth int
}

// Run performs the BFS LLDP discovery and renders the output.
// The log function receives human-readable status lines as discovery progresses.
// Cancel ctx to abort an in-progress scan.
func Run(ctx context.Context, cfg Config, log func(string)) (*graph.Topology, error) {
	// Parse ignore prefixes once; an empty list is valid (no filtering).
	var ignorePrefixes []*net.IPNet
	if len(cfg.IgnorePrefixStrs) > 0 {
		var err error
		ignorePrefixes, err = filter.ParseIgnorePrefixes(cfg.IgnorePrefixStrs)
		if err != nil {
			return nil, err
		}
	}

	baseCfg := snmpclient.Config{
		Port:      cfg.Port,
		Version:   cfg.Version,
		Community: cfg.Community,
		Username:  cfg.Username,
		AuthProto: snmpclient.AuthProto(cfg.AuthProto),
		AuthPass:  cfg.AuthPass,
		PrivProto: snmpclient.PrivProto(cfg.PrivProto),
		PrivPass:  cfg.PrivPass,
		SecLevel:  cfg.SecLevel,
		Timeout:   time.Duration(cfg.Timeout) * time.Second,
		Retries:   cfg.Retries,
	}

	topo := graph.New()
	queue := []queueItem{{host: cfg.SeedHost, depth: 0}}
	visited := map[string]bool{}

	log(fmt.Sprintf("Starting LLDP discovery from %s (max-hops=%d)", cfg.SeedHost, cfg.MaxHops))

	for len(queue) > 0 {
		// Check for cancellation before each device.
		select {
		case <-ctx.Done():
			log("Discovery cancelled.")
			break
		default:
		}
		if ctx.Err() != nil {
			break
		}

		item := queue[0]
		queue = queue[1:]

		if visited[item.host] {
			continue
		}
		visited[item.host] = true

		indent := fmt.Sprintf("%*s", item.depth*2, "")
		log(fmt.Sprintf("%s[hop %d] Querying %s...", indent, item.depth, item.host))

		snmpCfg := baseCfg
		snmpCfg.Host = item.host

		client, err := snmpclient.New(snmpCfg)
		if err != nil {
			log(fmt.Sprintf("%s  Warning: cannot connect to %s: %v", indent, item.host, err))
			continue
		}

		info, err := lldp.Walk(client)
		var ifAddrs []string
		if err == nil && cfg.ShowAddrs {
			raw, _ := lldp.WalkIPAddresses(client)
			ifAddrs = filter.Addrs(raw, cfg.AddrFamily, ignorePrefixes)
		}
		client.Close()

		if err != nil {
			log(fmt.Sprintf("%s  Warning: LLDP walk failed on %s: %v", indent, item.host, err))
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
			log(fmt.Sprintf("%s  No LLDP neighbors found.", indent))
			continue
		}

		log(fmt.Sprintf("%s  Found %d neighbor(s) on %s:", indent, len(info.Neighbors), localName))

		for _, neighbor := range info.Neighbors {
			if neighbor.RemoteSys == "" {
				continue
			}

			topo.AddNode(neighbor.RemoteSys, "")
			topo.AddEdge(localName, neighbor.RemoteSys, neighbor.LocalPort, neighbor.RemotePort)

			log(fmt.Sprintf("%s    %-30s  %s -> %s",
				indent, neighbor.RemoteSys, neighbor.LocalPort, neighbor.RemotePort))

			if item.depth >= cfg.MaxHops {
				log(fmt.Sprintf("%s      (max-hops reached, not recursing further)", indent))
				continue
			}

			if len(neighbor.MgmtAddrs) == 0 {
				log(fmt.Sprintf("%s      (no management address found — cannot recurse into this device)", indent))
				continue
			}

			queued := 0
			for _, ip := range filter.IPs(neighbor.MgmtAddrs, cfg.AddrFamily, ignorePrefixes) {
				ipStr := ip.String()
				if !visited[ipStr] && !inQueue(queue, ipStr) {
					queue = append(queue, queueItem{host: ipStr, depth: item.depth + 1})
					topo.AddNode(neighbor.RemoteSys, ipStr)
					log(fmt.Sprintf("%s      → queuing %s (%s)", indent, neighbor.RemoteSys, ipStr))
					queued++
				}
			}
			if queued == 0 {
				log(fmt.Sprintf("%s      (all %d address(es) already visited/queued)", indent, len(neighbor.MgmtAddrs)))
			}
		}
	}

	if len(topo.Nodes) == 0 {
		return nil, fmt.Errorf("no LLDP data discovered from %s\n"+
			"Check SNMP credentials and that LLDP is enabled on the device", cfg.SeedHost)
	}

	log(fmt.Sprintf("Discovered %d node(s), %d link(s)", len(topo.Nodes), len(topo.Edges)))

	if ctx.Err() != nil {
		// Cancelled after partial discovery — render what we have.
		log("Rendering partial results...")
	} else {
		log(fmt.Sprintf("Rendering %s -> %s...", cfg.OutputFormat, cfg.OutputFile))
	}

	if err := render.Render(topo, cfg.OutputFile, cfg.OutputFormat); err != nil {
		return nil, err
	}

	log(fmt.Sprintf("Done. Output saved to: %s", cfg.OutputFile))
	return topo, nil
}

func inQueue(queue []queueItem, host string) bool {
	for _, item := range queue {
		if item.host == host {
			return true
		}
	}
	return false
}
