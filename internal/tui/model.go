package tui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jowiho/zooxplorer/internal/format"
	"github.com/jowiho/zooxplorer/internal/snapshot"
)

type focusPane int

const (
	focusTree focusPane = iota
	focusContent
	metadataInnerHeight = 5
	aclInnerHeight      = 6
)

var metadataPathStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true)
var statsLabelStyle = lipgloss.NewStyle().Bold(true)
var statusBarStyle = lipgloss.NewStyle().Reverse(true)
var statusKeyStyle = lipgloss.NewStyle().Reverse(true).Bold(true)

type Model struct {
	tree          *snapshot.Tree
	selected      *snapshot.Node
	rows          []row
	sortOrder     sortColumn
	sortDesc      [5]bool
	expanded      map[string]bool
	treeOffset    int
	contentOffset int
	focus         focusPane
	statsOpen     bool
	statsText     string
	width         int
	height        int
}

func NewModel(tree *snapshot.Tree) Model {
	m := Model{
		tree:      tree,
		expanded:  make(map[string]bool),
		focus:     focusTree,
		sortOrder: sortByNodeName,
		sortDesc: [5]bool{
			sortByNodeName:    false,
			sortByNodeSize:    true,
			sortBySubtreeSize: true,
			sortByChildren:    true,
			sortByModified:    false,
		},
		width: 120,
	}
	if tree != nil {
		if len(tree.Root.Children) > 0 {
			m.selected = tree.Root.Children[0]
		} else {
			m.selected = tree.Root
		}
		m.refreshRows()
	}
	return m
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	needsRowRefresh := false
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		if m.statsOpen {
			m.statsOpen = false
			return m, nil
		}
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "ctrl+s":
			m.openStatsDialog()
			return m, nil
		case "ctrl+o":
			m.sortOrder = (m.sortOrder + 1) % 5
			needsRowRefresh = true
		case "ctrl+r":
			m.sortDesc[m.sortOrder] = !m.sortDesc[m.sortOrder]
			needsRowRefresh = true
		case "tab":
			if m.focus == focusTree {
				m.focus = focusContent
			} else {
				m.focus = focusTree
			}
		case "up":
			if m.focus == focusContent {
				m.scrollContent(-1)
			} else {
				m.moveSelection(-1)
			}
		case "alt+up", "meta+up":
			if m.focus == focusTree {
				m.selected = visibleParentNode(m.selected)
				m.contentOffset = 0
			}
		case "down":
			if m.focus == focusContent {
				m.scrollContent(1)
			} else {
				m.moveSelection(1)
			}
		case "left":
			if m.focus == focusTree && m.selected != nil {
				delete(m.expanded, m.selected.Path)
				needsRowRefresh = true
			}
		case "right":
			if m.focus == focusTree && m.selected != nil && len(m.selected.Children) > 0 {
				m.expanded[m.selected.Path] = true
				needsRowRefresh = true
			}
		}
	}
	if needsRowRefresh {
		m.refreshRows()
	}
	m.adjustTreeOffset()
	m.adjustContentOffset()
	return m, nil
}

func (m Model) View() string {
	if m.tree == nil || m.tree.Root == nil {
		return "No nodes to display.\n"
	}

	leftOuter, rightOuter, paneHeight := m.layout()
	totalWidth := leftOuter + 1 + rightOuter
	mainHeight := paneHeight - 1
	if mainHeight < 6 {
		mainHeight = 6
	}
	leftInner := leftOuter - 2
	rightInner := rightOuter - 2
	treeInnerHeight := mainHeight - 2

	treeLines := renderTreeWindow(m.rows, m.selected, leftInner, m.expanded, m.sortOrder, m.sortDesc[m.sortOrder], m.treeOffset, treeInnerHeight)
	treeStyle := lipgloss.NewStyle().Border(lipgloss.NormalBorder())
	if m.focus == focusTree {
		treeStyle = treeStyle.BorderForeground(lipgloss.Color("39"))
	}
	treeBox := treeStyle.
		Width(leftInner).
		Height(treeInnerHeight).
		Render(strings.Join(treeLines, "\n"))

	metaInnerHeight := metadataInnerHeight
	aclInner := aclInnerHeight
	contentInnerHeight := mainHeight - metaInnerHeight - aclInner - 8
	if contentInnerHeight < 1 {
		contentInnerHeight = 1
	}

	metadataLines := m.renderMetadataLines(rightInner, metaInnerHeight)
	metadataBox := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		Width(rightInner).
		Height(metaInnerHeight).
		Render(strings.Join(metadataLines, "\n"))

	aclLines := m.renderACLLines(rightInner, aclInner)
	aclBox := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		Width(rightInner).
		Render(strings.Join(aclLines, "\n"))

	contentLines := m.renderContentWindowLines(rightInner, contentInnerHeight)
	contentStyle := lipgloss.NewStyle().Border(lipgloss.NormalBorder())
	if m.focus == focusContent {
		contentStyle = contentStyle.BorderForeground(lipgloss.Color("39"))
	}
	contentBox := contentStyle.
		Width(rightInner).
		Height(contentInnerHeight).
		Render(strings.Join(contentLines, "\n"))

	rightPane := lipgloss.JoinVertical(lipgloss.Left, metadataBox, "", aclBox, "", contentBox)
	mainView := lipgloss.JoinHorizontal(lipgloss.Top, treeBox, " ", rightPane)
	statusBar := m.renderStatusBar(totalWidth)
	if !m.statsOpen {
		return mainView + "\n" + statusBar
	}
	overlay := lipgloss.Place(totalWidth, mainHeight, lipgloss.Center, lipgloss.Center, m.renderStatsDialog())
	return overlay + "\n" + statusBar
}

func (m *Model) refreshRows() {
	if m.tree == nil || m.tree.Root == nil {
		m.rows = nil
		return
	}
	m.rows = flatten(m.tree.Root, m.expanded, m.sortOrder, m.sortDesc[m.sortOrder])
}

func (m *Model) moveSelection(delta int) {
	if len(m.rows) == 0 || m.selected == nil {
		return
	}
	i := m.selectedRowIndex()
	if i == -1 {
		return
	}
	next := i + delta
	if next < 0 || next >= len(m.rows) {
		return
	}
	m.selected = m.rows[next].Node
	m.contentOffset = 0
}

func (m *Model) selectedRowIndex() int {
	for i := range m.rows {
		if m.rows[i].Node == m.selected {
			return i
		}
	}
	return -1
}

func (m Model) renderMetadata() string {
	if m.selected == nil {
		return ""
	}
	return fmt.Sprintf(
		"%s ID %d (version %d)\nMTime: %s\nCTime: %s\n%s\n%s",
		printablePath(m.selected.Path),
		m.selected.ACLRef,
		m.selected.Stat.Version,
		formatSnapshotTimeUTC(m.selected.Stat.Mtime),
		formatSnapshotTimeUTC(m.selected.Stat.Ctime),
		format.DataSizeSummary(m.selected.Data),
		nodeMetadata(m.selected),
	)
}

func (m Model) renderContent(width int) string {
	body := ""
	if m.selected != nil {
		body = format.ZNodeContent(m.selected.Data)
	}
	lines := strings.Split(body, "\n")
	for i := range lines {
		lines[i] = truncateANSI(lines[i], width)
	}
	return strings.Join(lines, "\n")
}

func formatSnapshotTimeUTC(epochMillis int64) string {
	return time.UnixMilli(epochMillis).UTC().Format(time.RFC3339)
}

func nodeMetadata(node *snapshot.Node) string {
	return fmt.Sprintf(
		"Metadata: czxid=%d mzxid=%d pzxid=%d child_version=%d ephOwner=%d",
		node.Stat.Czxid,
		node.Stat.Mzxid,
		node.Stat.Pzxid,
		node.Stat.Cversion,
		node.Stat.EphemeralOwner,
	)
}

func (m Model) renderMetadataLines(width, height int) []string {
	content := m.renderMetadata()
	lines := strings.Split(content, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	pathToken := ""
	if m.selected != nil {
		pathToken = printablePath(m.selected.Path)
	}
	for i := range lines {
		lines[i] = truncate(lines[i], width)
		if i == 0 && pathToken != "" {
			lines[i] = strings.Replace(lines[i], pathToken, metadataPathStyle.Render(pathToken), 1)
		}
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return lines
}

func (m Model) renderContentLines(width int) []string {
	content := m.renderContent(width)
	lines := strings.Split(content, "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	return lines
}

func (m Model) renderACL() string {
	if m.selected == nil {
		return "ACL ID: n/a\nACL Version: n/a\n\nNo node selected."
	}

	lines := []string{
		fmt.Sprintf("ACL ID %d (version %d)", m.selected.ACLRef, m.selected.Stat.Aversion),
		"",
	}

	if m.selected.ACLRef == -1 {
		lines = append(lines, "OPEN_ACL_UNSAFE")
		return strings.Join(lines, "\n")
	}
	if m.tree == nil || m.tree.ACLs == nil {
		lines = append(lines, "No ACL cache available.")
		return strings.Join(lines, "\n")
	}
	entries, ok := m.tree.ACLs[m.selected.ACLRef]
	if !ok || len(entries) == 0 {
		lines = append(lines, "No ACL entries found.")
		return strings.Join(lines, "\n")
	}
	for i, entry := range entries {
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, aclDetail(entry)))
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderACLLines(width, height int) []string {
	content := m.renderACL()
	lines := strings.Split(content, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for i := range lines {
		lines[i] = truncate(lines[i], width)
	}
	return lines
}

func aclDetail(entry snapshot.ACL) string {
	perms := formatACLPermissions(entry.Perms)
	switch entry.Scheme {
	case "digest":
		username := entry.ID
		if idx := strings.Index(username, ":"); idx >= 0 {
			username = username[:idx]
		}
		return fmt.Sprintf("%s: %s", username, perms)
	default:
		return fmt.Sprintf("scheme=%s id=%s perms=%s", entry.Scheme, entry.ID, perms)
	}
}

func formatACLPermissions(perms int32) string {
	if perms == 31 {
		return "all"
	}
	parts := make([]string, 0, 5)
	if perms&4 != 0 {
		parts = append(parts, "create")
	}
	if perms&1 != 0 {
		parts = append(parts, "read")
	}
	if perms&2 != 0 {
		parts = append(parts, "write")
	}
	if perms&8 != 0 {
		parts = append(parts, "delete")
	}
	if perms&16 != 0 {
		parts = append(parts, "admin")
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, "|")
}

func (m Model) renderContentWindowLines(width, height int) []string {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}

	lines := m.renderContentLines(width)
	needsScroll := len(lines) > height
	textWidth := width
	if needsScroll && width > 1 {
		textWidth = width - 1
	}

	offset := m.contentOffset
	maxOffset := len(lines) - height
	if maxOffset < 0 {
		maxOffset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}
	if offset < 0 {
		offset = 0
	}

	out := make([]string, 0, height)
	thumbPos, thumbSize := scrollbarPosition(height, len(lines), offset)
	for i := 0; i < height; i++ {
		idx := offset + i
		line := ""
		if idx >= 0 && idx < len(lines) {
			line = truncateANSI(lines[idx], textWidth)
		}
		line = padToWidthANSI(line, textWidth)
		if needsScroll {
			bar := "│"
			if i >= thumbPos && i < thumbPos+thumbSize {
				bar = "█"
			}
			line += bar
		}
		out = append(out, line)
	}
	return out
}

func (m *Model) adjustTreeOffset() {
	if len(m.rows) == 0 || m.selected == nil {
		m.treeOffset = 0
		return
	}
	_, _, paneHeight := m.layout()
	// Keep this in sync with View(): mainHeight = paneHeight - 1, treeInnerHeight = mainHeight - 2,
	// and one tree row is consumed by the table header.
	visibleHeight := paneHeight - 4
	if visibleHeight < 1 {
		visibleHeight = 1
	}

	sel := m.selectedRowIndex()
	if sel == -1 {
		return
	}

	if sel < m.treeOffset {
		m.treeOffset = sel
	}
	if sel >= m.treeOffset+visibleHeight {
		m.treeOffset = sel - visibleHeight + 1
	}

	maxOffset := len(m.rows) - visibleHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.treeOffset > maxOffset {
		m.treeOffset = maxOffset
	}
	if m.treeOffset < 0 {
		m.treeOffset = 0
	}
}

func (m *Model) scrollContent(delta int) {
	lines := m.renderContentLines(256)
	if len(lines) == 0 {
		m.contentOffset = 0
		return
	}
	_, _, paneHeight := m.layout()
	metaInnerHeight := metadataInnerHeight
	contentInnerHeight := paneHeight - metaInnerHeight - aclInnerHeight - 8
	if contentInnerHeight < 1 {
		contentInnerHeight = 1
	}
	maxOffset := len(lines) - contentInnerHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	next := m.contentOffset + delta
	if next < 0 {
		next = 0
	}
	if next > maxOffset {
		next = maxOffset
	}
	m.contentOffset = next
}

func (m *Model) adjustContentOffset() {
	lines := m.renderContentLines(256)
	_, _, paneHeight := m.layout()
	metaInnerHeight := metadataInnerHeight
	contentInnerHeight := paneHeight - metaInnerHeight - aclInnerHeight - 8
	if contentInnerHeight < 1 {
		contentInnerHeight = 1
	}
	maxOffset := len(lines) - contentInnerHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.contentOffset > maxOffset {
		m.contentOffset = maxOffset
	}
	if m.contentOffset < 0 {
		m.contentOffset = 0
	}
}

func scrollbarPosition(windowHeight, contentLen, offset int) (thumbPos int, thumbSize int) {
	if windowHeight <= 0 {
		return 0, 0
	}
	if contentLen <= windowHeight {
		return 0, windowHeight
	}
	thumbSize = (windowHeight * windowHeight) / contentLen
	if thumbSize < 1 {
		thumbSize = 1
	}
	maxThumbPos := windowHeight - thumbSize
	maxOffset := contentLen - windowHeight
	if maxOffset <= 0 {
		return 0, thumbSize
	}
	thumbPos = (offset * maxThumbPos) / maxOffset
	if thumbPos < 0 {
		thumbPos = 0
	}
	if thumbPos > maxThumbPos {
		thumbPos = maxThumbPos
	}
	return thumbPos, thumbSize
}

func (m Model) layout() (leftOuter, rightOuter, paneHeight int) {
	totalWidth := m.width
	if totalWidth < 64 {
		totalWidth = 64
	}
	paneHeight = m.height
	if paneHeight < 8 {
		paneHeight = 24
	}

	gap := 1
	leftOuter = (totalWidth - gap) / 2
	rightOuter = totalWidth - gap - leftOuter

	if leftOuter < 24 {
		leftOuter = 24
	}
	if rightOuter < 24 {
		rightOuter = 24
	}
	return leftOuter, rightOuter, paneHeight
}

func printablePath(path string) string {
	if path == "" {
		return "/"
	}
	return path
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}

func padToWidthANSI(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

func truncateANSI(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= max {
		return s
	}

	var b strings.Builder
	width := 0
	usedANSI := false
	for i := 0; i < len(s); {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			usedANSI = true
			j := i + 2
			for j < len(s) {
				c := s[j]
				if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
					j++
					break
				}
				j++
			}
			b.WriteString(s[i:j])
			i = j
			continue
		}

		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			break
		}
		rw := lipgloss.Width(string(r))
		if width+rw > max {
			break
		}
		b.WriteRune(r)
		width += rw
		i += size
	}
	if usedANSI {
		b.WriteString("\x1b[0m")
	}
	return b.String()
}

func (m Model) renderStatusBar(width int) string {
	text := strings.Join([]string{
		statusKeyStyle.Render("^C") + " Quit",
		statusKeyStyle.Render("^S") + " Show stats",
		statusKeyStyle.Render("Tab") + " Switch panels",
		statusKeyStyle.Render("^O") + " Change sort order",
		statusKeyStyle.Render("^R") + " Reverse sort order",
	}, " | ")
	if width < 1 {
		width = lipgloss.Width(text)
	}
	if width == 1 {
		return " "
	}
	line := text
	lineWidth := lipgloss.Width(line)
	innerWidth := width - 1
	if lineWidth < innerWidth {
		line += strings.Repeat(" ", innerWidth-lineWidth)
	} else if lineWidth > innerWidth {
		line = truncate(line, innerWidth)
	}
	return " " + statusBarStyle.Width(innerWidth).Render(line)
}

type snapshotStats struct {
	totalNodes     int
	ephemeralNodes int
	emptyNodes     int
	totalSize      int
	biggestSize    int
	biggestPath    string
}

func (m *Model) openStatsDialog() {
	stats := collectSnapshotStats(m.tree)
	avgSize := 0.0
	if stats.totalNodes > 0 {
		avgSize = float64(stats.totalSize) / float64(stats.totalNodes)
	}
	countLabels := []string{"Total nodes", "Ephemeral nodes", "Empty nodes"}
	labelWidth := maxLen(countLabels)
	countWidth := maxInt(
		len(strconv.Itoa(stats.totalNodes)),
		len(strconv.Itoa(stats.ephemeralNodes)),
		len(strconv.Itoa(stats.emptyNodes)),
	)
	avgRounded := int(math.Round(avgSize))
	sizeWidth := maxLen([]string{
		strconv.Itoa(avgRounded),
		strconv.Itoa(stats.biggestSize),
	})
	m.statsText = strings.Join([]string{
		"Snapshot Statistics",
		"",
		fmt.Sprintf("%-*s: %*d", labelWidth, "Total nodes", countWidth, stats.totalNodes),
		fmt.Sprintf("%-*s: %*d", labelWidth, "Ephemeral nodes", countWidth, stats.ephemeralNodes),
		fmt.Sprintf("%-*s: %*d", labelWidth, "Empty nodes", countWidth, stats.emptyNodes),
		"",
		fmt.Sprintf("Average node: %*d bytes", sizeWidth, avgRounded),
		fmt.Sprintf("Biggest node: %*d bytes at %s", sizeWidth, stats.biggestSize, stats.biggestPath),
		"",
		"Press any key to close.",
	}, "\n")
	m.statsOpen = true
}

func collectSnapshotStats(tree *snapshot.Tree) snapshotStats {
	stats := snapshotStats{biggestPath: "/"}
	if tree == nil || tree.Root == nil {
		return stats
	}

	var walk func(node *snapshot.Node)
	walk = func(node *snapshot.Node) {
		stats.totalNodes++
		size := len(node.Data)
		stats.totalSize += size
		if node.Stat.EphemeralOwner != 0 {
			stats.ephemeralNodes++
		}
		if size == 0 {
			stats.emptyNodes++
		}
		if size > stats.biggestSize {
			stats.biggestSize = size
			stats.biggestPath = printablePath(node.Path)
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(tree.Root)

	return stats
}

func (m Model) renderStatsDialog() string {
	totalWidth, _, _ := m.layout()
	dialogWidth := totalWidth - 2
	if dialogWidth < 32 {
		dialogWidth = 32
	}
	lines := strings.Split(m.statsText, "\n")
	for i := range lines {
		lines[i] = truncate(lines[i], dialogWidth)
		lines[i] = styleStatsLine(lines[i])
	}
	return lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color("214")).
		Width(dialogWidth).
		Render(strings.Join(lines, "\n"))
}

func styleStatsLine(line string) string {
	labels := map[string]struct{}{
		"Snapshot Statistics":     {},
		"Total nodes":             {},
		"Ephemeral nodes":         {},
		"Empty nodes":             {},
		"Average node":            {},
		"Biggest node":            {},
		"Press any key to close.": {},
	}
	if idx := strings.Index(line, ":"); idx > 0 {
		label := strings.TrimRight(line[:idx], " ")
		if _, ok := labels[label]; ok {
			return statsLabelStyle.Render(line[:idx]) + line[idx:]
		}
	}
	if _, ok := labels[line]; ok {
		return statsLabelStyle.Render(line)
	}
	return line
}

func maxInt(a, b, c int) int {
	m := a
	if b > m {
		m = b
	}
	if c > m {
		m = c
	}
	return m
}

func maxLen(values []string) int {
	max := 0
	for _, v := range values {
		if len(v) > max {
			max = len(v)
		}
	}
	return max
}
