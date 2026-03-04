package render

import (
	"fmt"
	"os"
	"strings"

	"github.com/buraglio/lldp2map/internal/graph"
)

// ExportDrawio writes the topology as a Draw.io (diagrams.net) XML file.
// Nodes are positioned with a circular layout; edges auto-route between them.
// The file can be opened directly in Draw.io or diagrams.net.
func ExportDrawio(topo *graph.Topology, outputFile string) error {
	positions := circularLayout(topo)

	f, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("create %s: %w", outputFile, err)
	}
	defer f.Close()

	w := func(format string, args ...any) {
		fmt.Fprintf(f, format, args...)
	}

	w("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	w("<mxGraphModel dx=\"1422\" dy=\"762\" grid=\"1\" gridSize=\"10\" guides=\"1\" ")
	w("tooltips=\"1\" connect=\"1\" arrows=\"1\" fold=\"1\" page=\"1\" pageScale=\"1\" ")
	w("pageWidth=\"1654\" pageHeight=\"1169\" math=\"0\" shadow=\"0\">\n")
	w("  <root>\n")
	w("    <mxCell id=\"0\" />\n")
	w("    <mxCell id=\"1\" parent=\"0\" />\n")

	// Node cells
	for name, node := range topo.Nodes {
		pos := positions[name]
		id := safeID("n-", name)
		label := dxEscape(nodeLabel(node))
		style := "rounded=1;whiteSpace=wrap;html=1;fillColor=#dae8fc;strokeColor=#6c8ebf;fontStyle=0;fontSize=11;"
		w("    <mxCell id=\"%s\" value=\"%s\" style=\"%s\" vertex=\"1\" parent=\"1\">\n",
			id, label, style)
		w("      <mxGeometry x=\"%.0f\" y=\"%.0f\" width=\"%.0f\" height=\"%.0f\" as=\"geometry\" />\n",
			pos.X, pos.Y, nodeW, nodeH)
		w("    </mxCell>\n")
	}

	// Edge cells — no value on the edge itself; port labels are separate floating text cells.
	portLabelStyle := "text;html=1;resizable=0;autosize=1;align=center;verticalAlign=middle;fontSize=9;strokeColor=none;fillColor=none;"
	for i, edge := range topo.Edges {
		id := fmt.Sprintf("e-%d", i)
		src := safeID("n-", edge.From)
		dst := safeID("n-", edge.To)
		style := "html=1;fontSize=9;endArrow=none;"
		w("    <mxCell id=\"%s\" value=\"\" style=\"%s\" edge=\"1\" source=\"%s\" target=\"%s\" parent=\"1\">\n",
			id, style, src, dst)
		w("      <mxGeometry relative=\"1\" as=\"geometry\" />\n")
		w("    </mxCell>\n")

		// Port labels placed at 25% (near From) and 75% (near To) along the straight
		// line between node centres — a good approximation even when Draw.io curves the edge.
		fromPos, fromOK := positions[edge.From]
		toPos, toOK := positions[edge.To]
		if fromOK && toOK {
			fc := nodeCenter(fromPos)
			tc := nodeCenter(toPos)
			if edge.FromPort != "" {
				x := fc.X + (tc.X-fc.X)*0.25 - 40
				y := fc.Y + (tc.Y-fc.Y)*0.25 - 10
				w("    <mxCell id=\"el-from-%d\" value=\"%s\" style=\"%s\" vertex=\"1\" parent=\"1\">\n",
					i, dxEscape(edge.FromPort), portLabelStyle)
				w("      <mxGeometry x=\"%.0f\" y=\"%.0f\" width=\"80\" height=\"20\" as=\"geometry\" />\n", x, y)
				w("    </mxCell>\n")
			}
			if edge.ToPort != "" {
				x := fc.X + (tc.X-fc.X)*0.75 - 40
				y := fc.Y + (tc.Y-fc.Y)*0.75 - 10
				w("    <mxCell id=\"el-to-%d\" value=\"%s\" style=\"%s\" vertex=\"1\" parent=\"1\">\n",
					i, dxEscape(edge.ToPort), portLabelStyle)
				w("      <mxGeometry x=\"%.0f\" y=\"%.0f\" width=\"80\" height=\"20\" as=\"geometry\" />\n", x, y)
				w("    </mxCell>\n")
			}
		}
	}

	w("  </root>\n")
	w("</mxGraphModel>\n")

	return nil
}

// dxEscape escapes a string for use as an XML attribute value in Draw.io.
// Newlines become &#xa; so Draw.io renders multi-line labels correctly.
func dxEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "\n", "&#xa;")
	return s
}
