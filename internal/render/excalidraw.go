package render

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/buraglio/lldp2map/internal/graph"
)

type excalidrawFile struct {
	Type     string            `json:"type"`
	Version  int               `json:"version"`
	Source   string            `json:"source"`
	Elements []map[string]any  `json:"elements"`
	AppState map[string]any    `json:"appState"`
	Files    map[string]any    `json:"files"`
}

// ExportExcalidraw writes the topology as an Excalidraw JSON file (.excalidraw).
// Nodes are rectangles with embedded text labels; edges are lines between node centres.
// Port labels are placed at the midpoint of each edge.
// The file can be imported directly into excalidraw.com or the desktop app.
func ExportExcalidraw(topo *graph.Topology, outputFile string) error {
	positions := circularLayout(topo)
	seed := 1
	nextSeed := func() int { s := seed; seed++; return s }

	var elements []map[string]any

	// Nodes: rectangle + contained text element
	for name, node := range topo.Nodes {
		pos := positions[name]
		nodeID := safeID("n-", name)
		textID := safeID("t-", name)
		label := nodeLabel(node)
		textH := float64(nodeLabelLineCount(node)) * 18.0

		rect := map[string]any{
			"id":              nodeID,
			"type":            "rectangle",
			"x":               pos.X,
			"y":               pos.Y,
			"width":           nodeW,
			"height":          nodeH,
			"angle":           0,
			"strokeColor":     "#1971c2",
			"backgroundColor": "#a5d8ff",
			"fillStyle":       "solid",
			"strokeWidth":     2,
			"strokeStyle":     "solid",
			"roughness":       0,
			"opacity":         100,
			"groupIds":        []string{},
			"frameId":         nil,
			"roundness":       map[string]any{"type": 3},
			"seed":            nextSeed(),
			"version":         1,
			"versionNonce":    nextSeed(),
			"isDeleted":       false,
			"boundElements":   []map[string]string{{"id": textID, "type": "text"}},
			"updated":         1,
			"link":            nil,
			"locked":          false,
		}

		text := map[string]any{
			"id":              textID,
			"type":            "text",
			"x":               pos.X,
			"y":               pos.Y + (nodeH-textH)/2,
			"width":           nodeW,
			"height":          textH,
			"angle":           0,
			"strokeColor":     "#1e1e1e",
			"backgroundColor": "transparent",
			"fillStyle":       "solid",
			"strokeWidth":     1,
			"strokeStyle":     "solid",
			"roughness":       0,
			"opacity":         100,
			"groupIds":        []string{},
			"frameId":         nil,
			"roundness":       nil,
			"seed":            nextSeed(),
			"version":         1,
			"versionNonce":    nextSeed(),
			"isDeleted":       false,
			"boundElements":   nil,
			"updated":         1,
			"link":            nil,
			"locked":          false,
			"text":            label,
			"fontSize":        12,
			"fontFamily":      1,
			"textAlign":       "center",
			"verticalAlign":   "middle",
			"containerId":     nodeID,
			"originalText":    label,
			"lineHeight":      1.25,
		}

		elements = append(elements, rect, text)
	}

	// Edges: line from node centre to node centre, optional label at midpoint
	for i, edge := range topo.Edges {
		fromPos, fromOK := positions[edge.From]
		toPos, toOK := positions[edge.To]
		if !fromOK || !toOK {
			continue
		}

		fc := nodeCenter(fromPos)
		tc := nodeCenter(toPos)
		dx := tc.X - fc.X
		dy := tc.Y - fc.Y

		line := map[string]any{
			"id":                  fmt.Sprintf("e-%d", i),
			"type":                "line",
			"x":                   fc.X,
			"y":                   fc.Y,
			"width":               dx,
			"height":              dy,
			"angle":               0,
			"strokeColor":         "#495057",
			"backgroundColor":     "transparent",
			"fillStyle":           "solid",
			"strokeWidth":         2,
			"strokeStyle":         "solid",
			"roughness":           0,
			"opacity":             100,
			"groupIds":            []string{},
			"frameId":             nil,
			"roundness":           map[string]any{"type": 2},
			"seed":                nextSeed(),
			"version":             1,
			"versionNonce":        nextSeed(),
			"isDeleted":           false,
			"boundElements":       nil,
			"updated":             1,
			"link":                nil,
			"locked":              false,
			"points":              [][2]float64{{0, 0}, {dx, dy}},
			"lastCommittedPoint":  [2]float64{dx, dy},
			"startBinding":        nil,
			"endBinding":          nil,
			"startArrowhead":      nil,
			"endArrowhead":        nil,
		}
		elements = append(elements, line)

		// Port labels: FromPort near the From device (~20%), ToPort near the To device (~80%).
		edgePortText := func(id, label string, frac float64) map[string]any {
			x := fc.X + dx*frac - 50
			y := fc.Y + dy*frac - 12
			return map[string]any{
				"id":              id,
				"type":            "text",
				"x":               x,
				"y":               y,
				"width":           100,
				"height":          16,
				"angle":           0,
				"strokeColor":     "#868e96",
				"backgroundColor": "transparent",
				"fillStyle":       "solid",
				"strokeWidth":     1,
				"strokeStyle":     "solid",
				"roughness":       0,
				"opacity":         80,
				"groupIds":        []string{},
				"frameId":         nil,
				"roundness":       nil,
				"seed":            nextSeed(),
				"version":         1,
				"versionNonce":    nextSeed(),
				"isDeleted":       false,
				"boundElements":   nil,
				"updated":         1,
				"link":            nil,
				"locked":          false,
				"text":            label,
				"fontSize":        10,
				"fontFamily":      1,
				"textAlign":       "center",
				"verticalAlign":   "middle",
				"containerId":     nil,
				"originalText":    label,
				"lineHeight":      1.25,
			}
		}
		if edge.FromPort != "" {
			elements = append(elements, edgePortText(fmt.Sprintf("et-from-%d", i), edge.FromPort, 0.2))
		}
		if edge.ToPort != "" {
			elements = append(elements, edgePortText(fmt.Sprintf("et-to-%d", i), edge.ToPort, 0.8))
		}
	}

	out := excalidrawFile{
		Type:     "excalidraw",
		Version:  2,
		Source:   "https://github.com/buraglio/lldp2map",
		Elements: elements,
		AppState: map[string]any{
			"gridSize":            nil,
			"viewBackgroundColor": "#ffffff",
		},
		Files: map[string]any{},
	}

	f, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("create %s: %w", outputFile, err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
