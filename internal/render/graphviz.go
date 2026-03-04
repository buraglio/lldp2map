package render

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/buraglio/lldp2map/internal/graph"
)

// GenerateDOT produces a Graphviz DOT representation of the topology.
func GenerateDOT(topo *graph.Topology) string {
	var sb strings.Builder

	sb.WriteString("graph network {\n")
	sb.WriteString("    layout=neato;\n")
	sb.WriteString("    overlap=false;\n")
	sb.WriteString("    splines=true;\n")
	sb.WriteString("    sep=\"+15\";\n")
	sb.WriteString("    node [shape=box, style=\"filled,rounded\", fillcolor=lightsteelblue, fontname=Helvetica, fontsize=10];\n")
	sb.WriteString("    edge [fontname=Helvetica, fontsize=8, color=gray40, labelfloat=true, labeldistance=2.5, labelangle=20];\n\n")

	for _, node := range topo.Nodes {
		sb.WriteString(fmt.Sprintf("    %q [label=%q];\n", node.Name, nodeLabel(node)))
	}

	sb.WriteString("\n")

	for _, edge := range topo.Edges {
		hasFrom := edge.FromPort != ""
		hasTo := edge.ToPort != ""
		if hasFrom || hasTo {
			sb.WriteString(fmt.Sprintf("    %q -- %q [taillabel=%q, headlabel=%q];\n",
				edge.From, edge.To, edge.FromPort, edge.ToPort))
		} else {
			sb.WriteString(fmt.Sprintf("    %q -- %q;\n", edge.From, edge.To))
		}
	}

	sb.WriteString("}\n")
	return sb.String()
}

// Render exports the topology to outputFile in the requested format.
// Supported formats: png, pdf, drawio, excalidraw.
// png and pdf require Graphviz to be installed (https://graphviz.org).
func Render(topo *graph.Topology, outputFile, format string) error {
	switch format {
	case "drawio":
		return ExportDrawio(topo, outputFile)
	case "excalidraw":
		return ExportExcalidraw(topo, outputFile)
	case "png", "pdf":
		return renderGraphviz(topo, outputFile, format)
	default:
		return fmt.Errorf("unsupported format %q: use png, pdf, drawio, or excalidraw", format)
	}
}

// renderGraphviz renders the topology via the Graphviz dot binary.
func renderGraphviz(topo *graph.Topology, outputFile, format string) error {
	if _, err := exec.LookPath("dot"); err != nil {
		return fmt.Errorf("graphviz 'dot' not found in PATH\nInstall it with: brew install graphviz  (macOS) or apt install graphviz (Linux)")
	}

	dot := GenerateDOT(topo)

	tmp, err := os.CreateTemp("", "lldp2map-*.dot")
	if err != nil {
		return fmt.Errorf("create temp DOT file: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.WriteString(dot); err != nil {
		tmp.Close()
		return fmt.Errorf("write DOT file: %w", err)
	}
	tmp.Close()

	cmd := exec.Command("dot", "-T"+format, tmp.Name(), "-o", outputFile)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("graphviz render failed: %w\n%s", err, strings.TrimSpace(string(out)))
	}

	return nil
}
