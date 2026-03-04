package graph

import "sync"

// Node represents a network device in the topology.
type Node struct {
	Name  string
	IP    string     // management IP used for SNMP targeting
	Addrs []string   // interface addresses shown in the diagram (--show-addrs)
}

// Edge represents a link between two devices with port label information.
type Edge struct {
	From      string
	To        string
	FromPort  string
	ToPort    string
}

// Topology holds the discovered network graph.
type Topology struct {
	mu    sync.Mutex
	Nodes map[string]*Node
	Edges []Edge
}

// New returns an empty Topology.
func New() *Topology {
	return &Topology{
		Nodes: make(map[string]*Node),
	}
}

// AddNode adds a node if it doesn't already exist.
// If the node exists and ip is non-empty, updates the IP.
func (t *Topology) AddNode(name, ip string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if n, exists := t.Nodes[name]; exists {
		if ip != "" && n.IP == "" {
			n.IP = ip
		}
		return
	}
	t.Nodes[name] = &Node{Name: name, IP: ip}
}

// AddEdge adds a link, skipping duplicates (including reversed direction).
func (t *Topology) AddEdge(from, to, fromPort, toPort string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, e := range t.Edges {
		if (e.From == from && e.To == to) || (e.From == to && e.To == from) {
			return
		}
	}
	t.Edges = append(t.Edges, Edge{From: from, To: to, FromPort: fromPort, ToPort: toPort})
}

// SetAddrs stores the interface address list for an existing node.
func (t *Topology) SetAddrs(name string, addrs []string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if n, ok := t.Nodes[name]; ok {
		n.Addrs = addrs
	}
}

// HasNode reports whether a node with the given name exists.
func (t *Topology) HasNode(name string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, ok := t.Nodes[name]
	return ok
}
