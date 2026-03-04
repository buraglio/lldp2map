package render

import (
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/buraglio/lldp2map/internal/graph"
)

const (
	nodeW   = 160.0
	nodeH   = 60.0
	padding = 80.0
)

// Point is a 2D coordinate.
type Point struct{ X, Y float64 }

// circularLayout assigns top-left positions to topology nodes arranged in a circle.
// Nodes are sorted for stable, reproducible output.
func circularLayout(topo *graph.Topology) map[string]Point {
	names := make([]string, 0, len(topo.Nodes))
	for name := range topo.Nodes {
		names = append(names, name)
	}
	sort.Strings(names)

	n := len(names)
	positions := make(map[string]Point, n)

	switch n {
	case 0:
		return positions
	case 1:
		positions[names[0]] = Point{X: 100, Y: 100}
		return positions
	}

	// Radius large enough that nodes don't overlap around the circumference.
	circumference := float64(n) * (nodeW + padding)
	radius := circumference / (2 * math.Pi)
	cx := radius + nodeW
	cy := radius + nodeH

	for i, name := range names {
		angle := 2*math.Pi*float64(i)/float64(n) - math.Pi/2
		positions[name] = Point{
			X: cx + radius*math.Cos(angle) - nodeW/2,
			Y: cy + radius*math.Sin(angle) - nodeH/2,
		}
	}

	return positions
}

// nodeCenter returns the centre point of a node box.
func nodeCenter(p Point) Point {
	return Point{X: p.X + nodeW/2, Y: p.Y + nodeH/2}
}

// nodeLabel builds the display text for a node.
// When Addrs is populated (--show-addrs), all interface addresses are listed.
// Otherwise, falls back to the management IP if known.
func nodeLabel(n *graph.Node) string {
	if len(n.Addrs) > 0 {
		return n.Name + "\n" + strings.Join(n.Addrs, "\n")
	}
	if n.IP != "" {
		return n.Name + "\n" + n.IP
	}
	return n.Name
}

// nodeLabelLineCount returns the number of display lines in a node label.
func nodeLabelLineCount(n *graph.Node) int {
	return strings.Count(nodeLabel(n), "\n") + 1
}

// nonAlnum matches characters that are not safe for XML/HTML IDs.
var nonAlnum = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// safeID returns a safe element ID by prefixing and replacing unsafe runes.
func safeID(prefix, name string) string {
	return prefix + nonAlnum.ReplaceAllString(name, "_")
}
