package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/jowiho/zooxplorer/internal/snapshot"
)

type row struct {
	Node  *snapshot.Node
	Depth int
}

type sortColumn int

const (
	sortByNodeName sortColumn = iota
	sortByNodeSize
	sortBySubtreeSize
	sortByChildren
	sortByModified
)

type treeMetrics struct {
	nodeSize    int
	subtreeSize int
}

func flatten(root *snapshot.Node, expanded map[string]bool, order sortColumn) []row {
	if root == nil {
		return nil
	}
	metrics := buildTreeMetrics(root)

	if order == sortByNodeSize || order == sortByModified {
		all := flattenAllNodes(root)
		sort.Slice(all, func(i, j int) bool {
			return lessNodes(all[i], all[j], order, metrics)
		})
		out := make([]row, 0, len(all))
		for _, node := range all {
			out = append(out, row{Node: node, Depth: 0})
		}
		return out
	}

	out := make([]row, 0, 256)
	var walk func(n *snapshot.Node, depth int)
	walk = func(n *snapshot.Node, depth int) {
		out = append(out, row{Node: n, Depth: depth})
		if !expanded[n.Path] {
			return
		}
		for _, child := range sortedChildren(n.Children, order, metrics) {
			walk(child, depth+1)
		}
	}

	// Root is implicit; the tree starts at top-level znodes.
	for _, child := range sortedChildren(root.Children, order, metrics) {
		walk(child, 0)
	}
	return out
}

func flattenAllNodes(root *snapshot.Node) []*snapshot.Node {
	out := make([]*snapshot.Node, 0, 256)
	var walk func(n *snapshot.Node)
	walk = func(n *snapshot.Node) {
		out = append(out, n)
		for _, child := range n.Children {
			walk(child)
		}
	}
	for _, child := range root.Children {
		walk(child)
	}
	return out
}

func sortedChildren(children []*snapshot.Node, order sortColumn, metrics map[*snapshot.Node]treeMetrics) []*snapshot.Node {
	sorted := make([]*snapshot.Node, len(children))
	copy(sorted, children)
	sort.Slice(sorted, func(i, j int) bool {
		return lessNodes(sorted[i], sorted[j], order, metrics)
	})
	return sorted
}

func lessNodes(left, right *snapshot.Node, order sortColumn, metrics map[*snapshot.Node]treeMetrics) bool {
	switch order {
	case sortByNodeSize:
		if len(left.Data) != len(right.Data) {
			return len(left.Data) > len(right.Data)
		}
	case sortBySubtreeSize:
		if metrics[left].subtreeSize != metrics[right].subtreeSize {
			return metrics[left].subtreeSize > metrics[right].subtreeSize
		}
	case sortByChildren:
		if len(left.Children) != len(right.Children) {
			return len(left.Children) > len(right.Children)
		}
	case sortByModified:
		if left.Stat.Mtime != right.Stat.Mtime {
			return left.Stat.Mtime < right.Stat.Mtime
		}
	default:
		if left.ID != right.ID {
			return left.ID < right.ID
		}
	}

	if left.ID != right.ID {
		return left.ID < right.ID
	}
	return left.Path < right.Path
}

func isFlatMode(order sortColumn) bool {
	return order == sortByNodeSize || order == sortByModified
}

func isDescendingSort(order sortColumn) bool {
	return order == sortByNodeSize || order == sortBySubtreeSize || order == sortByChildren
}

var (
	treeNodeNameStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true)
	selectedRowStyle  = lipgloss.NewStyle().Reverse(true)
	treeHeaderStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
)

func renderTree(rows []row, selected *snapshot.Node, width int, expanded map[string]bool, order sortColumn) string {
	lines := renderTreeWindow(rows, selected, width, expanded, order, 0, len(rows))
	return strings.Join(lines, "\n")
}

func renderTreeWindow(rows []row, selected *snapshot.Node, width int, expanded map[string]bool, order sortColumn, offset, height int) []string {
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
	lines = append(lines, treeHeaderStyle.Render(formatTreeTableHeader(width, order)))
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
		if !isFlatMode(order) {
			if len(r.Node.Children) > 0 {
				icon = "+"
				if expanded[r.Node.Path] {
					icon = "-"
				}
			}
		}
		sizeInfo := sizeLabel(metrics[r.Node])
		plainPrefix := prefix
		displayName := fmt.Sprintf("%s%s%s %s", plainPrefix, indent, icon, r.Node.ID)
		line := formatTreeTableRow(displayName, sizeInfo, metrics[r.Node].subtreeSize, len(r.Node.Children), r.Node.Stat.Mtime, width)
		if selected == r.Node {
			line = selectedRowStyle.Width(width).Render(padToWidth(line, width))
		} else {
			line = strings.Replace(line, r.Node.ID, treeNodeNameStyle.Render(r.Node.ID), 1)
		}
		lines = append(lines, line)
	}
	return lines
}

func formatTreeTableHeader(width int, order sortColumn) string {
	nameW, nodeW, subtreeW, childW, modifiedW := tableColumnWidths(width)
	return fmt.Sprintf(
		"%-*s %*s %*s %*s %*s",
		nameW,
		sortedHeaderLabel("Node name", sortByNodeName, order),
		nodeW,
		sortedHeaderLabel("Node size", sortByNodeSize, order),
		subtreeW,
		sortedHeaderLabel("Subtree size", sortBySubtreeSize, order),
		childW,
		sortedHeaderLabel("Children", sortByChildren, order),
		modifiedW,
		sortedHeaderLabel("Modified", sortByModified, order),
	)
}

func sortedHeaderLabel(label string, col, active sortColumn) string {
	if col == active {
		if isDescendingSort(col) {
			return "▼ " + label
		}
		return "▲ " + label
	}
	return "  " + label
}

func formatTreeTableRow(name string, nodeSizeLabel string, subtreeSize, childCount int, mtime int64, width int) string {
	nameW, nodeW, subtreeW, childW, modifiedW := tableColumnWidths(width)
	return fmt.Sprintf(
		"%-*s %*s %*d %*d %-*s",
		nameW,
		truncate(name, nameW),
		nodeW,
		nodeSizeLabel,
		subtreeW,
		subtreeSize,
		childW,
		childCount,
		modifiedW,
		formatMTimeISO(mtime),
	)
}

func tableColumnWidths(width int) (nameW, nodeW, subtreeW, childW, modifiedW int) {
	nodeW = 11    // "  Node size"
	subtreeW = 14 // "  Subtree size"
	childW = 10   // "  Children"
	modifiedW = 20
	nameW = width - (nodeW + subtreeW + childW + modifiedW + 4)
	if nameW < 11 { // "  Node name"
		nameW = 11
	}
	return nameW, nodeW, subtreeW, childW, modifiedW
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

func formatMTimeISO(millis int64) string {
	return time.UnixMilli(millis).UTC().Format(time.RFC3339)
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

func buildTreeMetrics(root *snapshot.Node) map[*snapshot.Node]treeMetrics {
	metrics := make(map[*snapshot.Node]treeMetrics)
	var fill func(node *snapshot.Node) treeMetrics
	fill = func(node *snapshot.Node) treeMetrics {
		if m, ok := metrics[node]; ok {
			return m
		}
		total := len(node.Data)
		for _, child := range node.Children {
			total += fill(child).subtreeSize
		}
		m := treeMetrics{nodeSize: len(node.Data), subtreeSize: total}
		metrics[node] = m
		return m
	}
	fill(root)
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
