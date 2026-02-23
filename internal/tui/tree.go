package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/jowiho/zooxplorer/internal/snapshot"
)

type row struct {
	Node  *snapshot.Node
	Depth int
}

type treeMetrics struct {
	nodeSize    int
	subtreeSize int
}

func flatten(root *snapshot.Node, expanded map[string]bool) []row {
	if root == nil {
		return nil
	}
	out := make([]row, 0, 256)
	var walk func(n *snapshot.Node, depth int)
	walk = func(n *snapshot.Node, depth int) {
		out = append(out, row{Node: n, Depth: depth})
		if !expanded[n.Path] {
			return
		}
		for _, child := range sortedChildren(n.Children) {
			walk(child, depth+1)
		}
	}

	// Root is implicit; the tree starts at top-level znodes.
	for _, child := range sortedChildren(root.Children) {
		walk(child, 0)
	}
	return out
}

func sortedChildren(children []*snapshot.Node) []*snapshot.Node {
	sorted := make([]*snapshot.Node, len(children))
	copy(sorted, children)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].ID == sorted[j].ID {
			return sorted[i].Path < sorted[j].Path
		}
		return sorted[i].ID < sorted[j].ID
	})
	return sorted
}

var (
	treeNodeNameStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true)
	selectedRowStyle  = lipgloss.NewStyle().Reverse(true)
	treeHeaderStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
)

func renderTree(rows []row, selected *snapshot.Node, width int, expanded map[string]bool) string {
	lines := renderTreeWindow(rows, selected, width, expanded, 0, len(rows))
	return strings.Join(lines, "\n")
}

func renderTreeWindow(rows []row, selected *snapshot.Node, width int, expanded map[string]bool, offset, height int) []string {
	if width < 10 {
		width = 10
	}
	if height < 1 {
		height = 1
	}
	if offset < 0 {
		offset = 0
	}
	maxOffset := len(rows) - height
	if maxOffset < 0 {
		maxOffset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}

	metrics := computeTreeMetrics(rows)
	lines := make([]string, 0, height)
	lines = append(lines, treeHeaderStyle.Render(formatTreeTableHeader(width)))
	dataHeight := height - 1
	if dataHeight < 0 {
		dataHeight = 0
	}
	for i := 0; i < dataHeight; i++ {
		idx := offset + i
		if idx >= len(rows) {
			lines = append(lines, "")
			continue
		}
		r := rows[idx]
		prefix := "  "
		if selected == r.Node {
			prefix = "> "
		}
		indent := strings.Repeat("  ", r.Depth)
		icon := " "
		if len(r.Node.Children) > 0 {
			icon = "+"
			if expanded[r.Node.Path] {
				icon = "-"
			}
		}
		sizeInfo := sizeLabel(metrics[r.Node])
		plainPrefix := prefix
		displayName := fmt.Sprintf("%s%s%s %s", plainPrefix, indent, icon, r.Node.ID)
		line := formatTreeTableRow(displayName, sizeInfo, metrics[r.Node].subtreeSize, len(r.Node.Children), width)
		if selected == r.Node {
			line = selectedRowStyle.Width(width).Render(padToWidth(line, width))
		} else {
			line = strings.Replace(line, r.Node.ID, treeNodeNameStyle.Render(r.Node.ID), 1)
		}
		lines = append(lines, line)
	}
	return lines
}

func formatTreeTableHeader(width int) string {
	nameW, nodeW, subtreeW, childW := tableColumnWidths(width)
	return fmt.Sprintf(
		"%-*s %*s %*s %*s",
		nameW,
		"Node name",
		nodeW,
		"Node size",
		subtreeW,
		"Subtree size",
		childW,
		"Child count",
	)
}

func formatTreeTableRow(name string, nodeSizeLabel string, subtreeSize, childCount int, width int) string {
	nameW, nodeW, subtreeW, childW := tableColumnWidths(width)
	return fmt.Sprintf(
		"%-*s %*s %*d %*d",
		nameW,
		truncate(name, nameW),
		nodeW,
		nodeSizeLabel,
		subtreeW,
		subtreeSize,
		childW,
		childCount,
	)
}

func tableColumnWidths(width int) (nameW, nodeW, subtreeW, childW int) {
	nodeW = 10
	subtreeW = 12
	childW = 11
	nameW = width - (nodeW + subtreeW + childW + 3)
	if nameW < 8 {
		nameW = 8
	}
	return nameW, nodeW, subtreeW, childW
}

func padToWidth(s string, width int) string {
	if width <= 0 {
		return s
	}
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

func computeTreeMetrics(rows []row) map[*snapshot.Node]treeMetrics {
	metrics := make(map[*snapshot.Node]treeMetrics, len(rows))
	var fill func(node *snapshot.Node) treeMetrics
	fill = func(node *snapshot.Node) treeMetrics {
		if m, ok := metrics[node]; ok {
			return m
		}
		total := len(node.Data)
		for _, child := range node.Children {
			total += fill(child).subtreeSize
		}
		m := treeMetrics{
			nodeSize:    len(node.Data),
			subtreeSize: total,
		}
		metrics[node] = m
		return m
	}
	for _, r := range rows {
		fill(r.Node)
	}
	return metrics
}

func sizeLabel(m treeMetrics) string {
	return fmt.Sprintf("%d", m.nodeSize)
}

func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	if max <= 1 {
		return s[:max]
	}
	return s[:max-1] + "…"
}
