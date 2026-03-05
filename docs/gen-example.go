//go:build ignore

// gen-example is a dumb program that creates a crude example topology diagram embedded in the README.
// Run with: go run docs/gen-example.go
package main

import (
	"log"

	"github.com/buraglio/lldp2map/internal/graph"
	"github.com/buraglio/lldp2map/internal/render"
)

func main() {
	topo := graph.New()

	// Nodes — mixed-vendor campus topology
	// Addresses use RFC 9637 documentation prefix (3fff::/20)
	// Also support legacy IP
	topo.AddNode("core-router", "3fff:ffff::1")
	topo.AddNode("edge-router-1", "3fff:0:1::1")
	topo.AddNode("edge-router-2", "3fff:0:1::2")
	topo.AddNode("dist-sw", "3fff:0:2::1")
	topo.AddNode("access-sw-1", "3fff:0:3::1")
	topo.AddNode("access-sw-2", "3fff:0:3::2")
	topo.AddNode("oob-sw", "3fff:0:4::1")

	// Interface addresses to show how the --show-addrs behaviour works
	topo.SetAddrs("core-router", []string{"3fff:ffff::1", "3fff:ffff::2"})

	// Show edge devs with port labels
	topo.AddEdge("core-router", "edge-router-1", "eth0", "Gi0/0")
	topo.AddEdge("core-router", "edge-router-2", "eth1", "Gi0/0")
	topo.AddEdge("edge-router-1", "dist-sw", "Gi0/1", "Te1/1")
	topo.AddEdge("edge-router-2", "dist-sw", "Gi0/2", "Te1/2")
	topo.AddEdge("dist-sw", "access-sw-1", "Gi1/1", "uplink1")
	topo.AddEdge("dist-sw", "access-sw-2", "Gi1/2", "uplink1")
	topo.AddEdge("core-router", "oob-sw", "mgmt0", "Gi24")

	if err := render.Render(topo, "docs/example.png", "png"); err != nil {
		log.Fatalf("render: %v", err)
	}
	log.Println("wrote docs/example.png")
}
