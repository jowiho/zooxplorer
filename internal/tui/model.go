package tui

import (
	"fmt"
	"math"
	"os/exec"
	"regexp"
	"runtime"
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
)

type searchScope int

const (
	searchNodes searchScope = iota
	searchContent
)

type searchDoneMsg struct {
	scope        searchScope
	query        string
	found        bool
	node         *snapshot.Node
	nameMatched  bool
	contentMatch int
}

type searchSpinnerMsg struct{}

var metadataPathStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true)
var statsLabelStyle = lipgloss.NewStyle().Bold(true)
var statusBarStyle = lipgloss.NewStyle().Reverse(true)
var statusKeyStyle = lipgloss.NewStyle().Reverse(true).Bold(true)
var contentSelectionStyle = lipgloss.NewStyle().Reverse(true)
var searchMatchStyle = lipgloss.NewStyle().Background(lipgloss.Color("226")).Foreground(lipgloss.Color("0"))
var searchInputStyle = lipgloss.NewStyle().Reverse(true)
var ansiEscapeRE = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

type Model struct {
	tree                  *snapshot.Tree
	selected              *snapshot.Node
	rows                  []row
	rowIndex              map[*snapshot.Node]int
	metrics               map[*snapshot.Node]treeMetrics
	sortOrder             sortColumn
	sortDesc              [5]bool
	expanded              map[string]bool
	treeOffset            int
	contentOffset         int
	contentLines          []string
	contentNode           *snapshot.Node
	contentSelect         bool
	copyContent           func(string) error
	searchOpen            bool
	searchScope           searchScope
	searchInput           string
	searchFirstKeyPending bool
	searchRunning         bool
	searchMessage         string
	searchSpinStep        int
	lastNodeQuery         string
	lastBodyQuery         string
	matchQuery            string
	matchIndex            int
	matchNode             *snapshot.Node
	nodeMatchQuery        string
	nodeMatchNode         *snapshot.Node
	focus                 focusPane
	statsOpen             bool
	statsText             string
	width                 int
	height                int
}

func NewModel(tree *snapshot.Tree) Model {
	m := Model{
		tree:      tree,
		rowIndex:  make(map[*snapshot.Node]int),
		metrics:   make(map[*snapshot.Node]treeMetrics),
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
		copyContent: func(s string) error {
			return copyToClipboard(s)
		},
		matchIndex: -1,
	}
	if tree != nil {
		if len(tree.Root.Children) > 0 {
			m.selected = tree.Root.Children[0]
		} else {
			m.selected = tree.Root
		}
		m.metrics = buildTreeMetrics(tree.Root)
		m.refreshRows()
		m.refreshContentLines()
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
	case searchSpinnerMsg:
		if m.searchRunning {
			m.searchSpinStep = (m.searchSpinStep + 1) % 4
			return m, searchSpinnerTickCmd()
		}
	case searchDoneMsg:
		m.searchRunning = false
		if !msg.found {
			m.searchMessage = fmt.Sprintf("No results for %q", msg.query)
			return m, nil
		}
		m.searchMessage = ""
		if msg.scope == searchNodes {
			m.selectNode(msg.node)
			m.centerSelectedRowInTree()
			m.nodeMatchQuery = msg.query
			m.nodeMatchNode = msg.node
			if msg.contentMatch >= 0 {
				m.matchQuery = msg.query
				m.matchIndex = msg.contentMatch
				m.matchNode = msg.node
				m.scrollMatchIntoView()
			} else {
				m.clearContentMatch()
			}
		} else {
			m.matchQuery = msg.query
			m.matchIndex = msg.contentMatch
			m.matchNode = m.selected
			m.scrollMatchIntoView()
			m.clearNodeMatch()
		}
		m.searchOpen = false
		m.searchFirstKeyPending = false
		return m, nil
	case tea.KeyMsg:
		if m.searchOpen {
			if m.searchRunning {
				if msg.String() == "ctrl+q" {
					return m, tea.Quit
				}
				return m, nil
			}
			switch msg.String() {
			case "ctrl+q":
				return m, tea.Quit
			case "esc":
				m.searchOpen = false
				m.searchFirstKeyPending = false
				m.searchMessage = ""
				return m, nil
			case "enter":
				query := m.searchInput
				m.searchFirstKeyPending = false
				if query != "" {
					if m.searchScope == searchNodes {
						m.lastNodeQuery = query
						m.searchRunning = true
						m.searchMessage = ""
						return m, tea.Batch(m.startNodeSearchCmd(query), searchSpinnerTickCmd())
					} else {
						m.lastBodyQuery = query
						m.searchRunning = true
						m.searchMessage = ""
						return m, tea.Batch(m.startContentSearchCmd(query), searchSpinnerTickCmd())
					}
				}
				m.searchOpen = false
				return m, nil
			case "backspace", "ctrl+h":
				if m.searchFirstKeyPending {
					m.searchInput = ""
					m.searchFirstKeyPending = false
				} else {
					r := []rune(m.searchInput)
					if len(r) > 0 {
						m.searchInput = string(r[:len(r)-1])
					}
				}
				m.searchMessage = ""
				return m, nil
			}
			if msg.Type == tea.KeyRunes {
				m.searchInput += string(msg.Runes)
				m.searchFirstKeyPending = false
				m.searchMessage = ""
				return m, nil
			}
			return m, nil
		}
		if m.statsOpen {
			m.statsOpen = false
			return m, nil
		}
		switch msg.String() {
		case "ctrl+q":
			return m, tea.Quit
		case "ctrl+c":
			if m.focus == focusContent {
				if m.contentSelect && m.copyContent != nil {
					_ = m.copyContent(m.selectedContentText())
				}
			}
			return m, nil
		case "ctrl+a":
			if m.focus == focusContent {
				m.contentSelect = true
			}
			return m, nil
		case "ctrl+s":
			m.openStatsDialog()
			return m, nil
		case "ctrl+f":
			m.searchOpen = true
			m.searchFirstKeyPending = true
			m.searchRunning = false
			m.searchMessage = ""
			if m.focus == focusContent {
				m.searchScope = searchContent
				m.searchInput = m.lastBodyQuery
			} else {
				m.searchScope = searchNodes
				m.searchInput = m.lastNodeQuery
			}
			return m, nil
		case "ctrl+o":
			m.sortOrder = (m.sortOrder + 1) % 5
			if !isFlatMode(m.sortOrder) {
				m.expandSelectedAncestors()
			}
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
			m.contentSelect = false
		case "up":
			if m.focus == focusContent {
				m.scrollContent(-1)
			} else {
				m.moveSelection(-1)
			}
		case "pgup":
			if m.focus == focusTree {
				m.moveSelectionPage(-1)
			}
		case "alt+up", "meta+up":
			if m.focus == focusTree {
				m.selected = visibleParentNode(m.selected)
				m.contentOffset = 0
				m.refreshContentLines()
			}
		case "down":
			if m.focus == focusContent {
				m.scrollContent(1)
			} else {
				m.moveSelection(1)
			}
		case "pgdown":
			if m.focus == focusTree {
				m.moveSelectionPage(1)
			}
		case "home":
			if m.focus == focusTree {
				m.moveSelectionToBoundary(true)
			}
		case "end":
			if m.focus == focusTree {
				m.moveSelectionToBoundary(false)
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
	if _, ok := msg.(tea.WindowSizeMsg); ok {
		m.adjustContentOffset()
	}
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

	treeLines := renderTreeWindow(
		m.rows,
		m.selected,
		leftInner,
		m.expanded,
		m.sortOrder,
		m.sortDesc[m.sortOrder],
		m.metrics,
		m.nodeMatchNode,
		m.nodeMatchQuery,
		m.treeOffset,
		treeInnerHeight,
	)
	treeStyle := lipgloss.NewStyle().Border(lipgloss.NormalBorder())
	if m.focus == focusTree {
		treeStyle = treeStyle.BorderForeground(lipgloss.Color("39"))
	}
	treeBox := treeStyle.
		Width(leftInner).
		Height(treeInnerHeight).
		Render(strings.Join(treeLines, "\n"))

	metaInnerHeight := metadataInnerHeight
	aclInner := m.aclInnerHeight(rightInner, mainHeight)
	contentInnerHeight := m.contentInnerHeight()

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

	rightPane := lipgloss.JoinVertical(lipgloss.Left, metadataBox, aclBox, contentBox)
	mainView := lipgloss.JoinHorizontal(lipgloss.Top, treeBox, " ", rightPane)
	statusBar := m.renderStatusBar(totalWidth)
	if !m.statsOpen {
		if !m.searchOpen {
			return mainView + "\n" + statusBar
		}
		overlay := lipgloss.Place(totalWidth, mainHeight, lipgloss.Center, lipgloss.Center, m.renderSearchDialog(totalWidth))
		return overlay + "\n" + statusBar
	}
	overlay := lipgloss.Place(totalWidth, mainHeight, lipgloss.Center, lipgloss.Center, m.renderStatsDialog())
	return overlay + "\n" + statusBar
}

func (m *Model) refreshRows() {
	if m.tree == nil || m.tree.Root == nil {
		m.rows = nil
		m.rowIndex = map[*snapshot.Node]int{}
		m.metrics = map[*snapshot.Node]treeMetrics{}
		return
	}
	if len(m.metrics) == 0 {
		m.metrics = buildTreeMetrics(m.tree.Root)
	}
	m.rows = flatten(m.tree.Root, m.expanded, m.sortOrder, m.sortDesc[m.sortOrder], m.metrics)
	idx := make(map[*snapshot.Node]int, len(m.rows))
	for i := range m.rows {
		idx[m.rows[i].Node] = i
	}
	m.rowIndex = idx
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
	m.contentSelect = false
	m.clearNodeMatch()
	m.clearContentMatch()
	m.refreshContentLines()
}

func (m *Model) moveSelectionPage(direction int) {
	if len(m.rows) == 0 || m.selected == nil {
		return
	}
	step := m.treeVisibleDataRows()
	if step < 1 {
		step = 1
	}
	if direction < 0 {
		m.moveSelection(-step)
		return
	}
	m.moveSelection(step)
}

func (m *Model) moveSelectionToBoundary(toStart bool) {
	if len(m.rows) == 0 {
		return
	}
	if toStart {
		m.selected = m.rows[0].Node
	} else {
		m.selected = m.rows[len(m.rows)-1].Node
	}
	m.contentOffset = 0
	m.contentSelect = false
	m.clearNodeMatch()
	m.clearContentMatch()
	m.refreshContentLines()
}

func (m *Model) selectedRowIndex() int {
	if m.rowIndex == nil || m.selected == nil {
		return -1
	}
	if i, ok := m.rowIndex[m.selected]; ok {
		return i
	}
	return -1
}

func (m *Model) expandSelectedAncestors() {
	if m.selected == nil {
		return
	}
	for p := m.selected.Parent; p != nil && p.Parent != nil; p = p.Parent {
		m.expanded[p.Path] = true
	}
}

func (m *Model) refreshContentLines() {
	if m.selected == nil {
		m.contentNode = nil
		m.contentLines = nil
		return
	}
	if m.contentNode == m.selected {
		return
	}
	m.contentNode = m.selected
	body := format.ZNodeContent(m.selected.Data)
	lines := strings.Split(body, "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = nil
	}
	m.contentLines = lines
}

func (m Model) selectedContentText() string {
	if len(m.contentLines) == 0 {
		return ""
	}
	return ansiEscapeRE.ReplaceAllString(strings.Join(m.contentLines, "\n"), "")
}

func (m *Model) clearContentMatch() {
	m.matchQuery = ""
	m.matchIndex = -1
	m.matchNode = nil
}

func (m *Model) clearNodeMatch() {
	m.nodeMatchQuery = ""
	m.nodeMatchNode = nil
}

func (m Model) startNodeSearchCmd(query string) tea.Cmd {
	tree := m.tree
	metrics := m.metrics
	selected := m.selected
	return func() tea.Msg {
		nodes := depthFirstNodesByName(tree, metrics)
		if len(nodes) == 0 {
			return searchDoneMsg{scope: searchNodes, query: query, found: false, contentMatch: -1}
		}
		start := -1
		for i, n := range nodes {
			if n == selected {
				start = i
				break
			}
		}
		for step := 1; step <= len(nodes); step++ {
			idx := (start + step) % len(nodes)
			node := nodes[idx]
			if strings.Contains(node.ID, query) {
				return searchDoneMsg{
					scope:        searchNodes,
					query:        query,
					found:        true,
					node:         node,
					nameMatched:  true,
					contentMatch: -1,
				}
			}
			if matchAt := strings.Index(plainNodeContent(node), query); matchAt >= 0 {
				return searchDoneMsg{
					scope:        searchNodes,
					query:        query,
					found:        true,
					node:         node,
					contentMatch: matchAt,
				}
			}
		}
		return searchDoneMsg{scope: searchNodes, query: query, found: false, contentMatch: -1}
	}
}

func (m Model) startContentSearchCmd(query string) tea.Cmd {
	selected := m.selected
	matchNode := m.matchNode
	matchQuery := m.matchQuery
	matchIndex := m.matchIndex
	return func() tea.Msg {
		if selected == nil {
			return searchDoneMsg{scope: searchContent, query: query, found: false, contentMatch: -1}
		}
		text := plainNodeContent(selected)
		if text == "" {
			return searchDoneMsg{scope: searchContent, query: query, found: false, contentMatch: -1}
		}
		start := 0
		if matchQuery == query && matchNode == selected && matchIndex >= 0 {
			start = matchIndex + len(query)
			if start > len(text) {
				start = len(text)
			}
		}
		foundAt := -1
		if start < len(text) {
			if idx := strings.Index(text[start:], query); idx >= 0 {
				foundAt = start + idx
			}
		}
		if foundAt < 0 {
			foundAt = strings.Index(text, query)
		}
		if foundAt < 0 {
			return searchDoneMsg{scope: searchContent, query: query, found: false, contentMatch: -1}
		}
		return searchDoneMsg{scope: searchContent, query: query, found: true, node: selected, contentMatch: foundAt}
	}
}

func (m *Model) selectNode(node *snapshot.Node) {
	if node == nil {
		return
	}
	m.selected = node
	m.contentOffset = 0
	m.contentSelect = false
	m.expandSelectedAncestors()
	m.refreshRows()
	m.refreshContentLines()
	m.adjustTreeOffset()
}

func (m *Model) centerSelectedRowInTree() {
	if len(m.rows) == 0 {
		m.treeOffset = 0
		return
	}
	sel := m.selectedRowIndex()
	if sel < 0 {
		return
	}
	visible := m.treeVisibleDataRows()
	if visible < 1 {
		visible = 1
	}
	target := sel - visible/2
	if target < 0 {
		target = 0
	}
	maxOffset := len(m.rows) - visible
	if maxOffset < 0 {
		maxOffset = 0
	}
	if target > maxOffset {
		target = maxOffset
	}
	m.treeOffset = target
}

func depthFirstNodesByName(tree *snapshot.Tree, metrics map[*snapshot.Node]treeMetrics) []*snapshot.Node {
	if tree == nil || tree.Root == nil {
		return nil
	}
	if len(metrics) == 0 {
		return flattenAllNodes(tree.Root)
	}
	out := make([]*snapshot.Node, 0, 256)
	var walk func(node *snapshot.Node)
	walk = func(node *snapshot.Node) {
		out = append(out, node)
		for _, child := range sortedChildren(node.Children, sortByNodeName, false, metrics) {
			walk(child)
		}
	}
	for _, child := range sortedChildren(tree.Root.Children, sortByNodeName, false, metrics) {
		walk(child)
	}
	return out
}

func plainNodeContent(node *snapshot.Node) string {
	if node == nil {
		return ""
	}
	return ansiEscapeRE.ReplaceAllString(format.ZNodeContent(node.Data), "")
}

func searchSpinnerTickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
		return searchSpinnerMsg{}
	})
}

func (m *Model) scrollMatchIntoView() {
	if m.matchNode != m.selected || m.matchQuery == "" || m.matchIndex < 0 {
		return
	}
	text := m.selectedContentText()
	if m.matchIndex > len(text) {
		return
	}
	line := strings.Count(text[:m.matchIndex], "\n")
	height := m.contentInnerHeight()
	if height < 1 {
		height = 1
	}
	target := line - height/2
	if target < 0 {
		target = 0
	}
	maxOffset := len(m.contentLines) - height
	if maxOffset < 0 {
		maxOffset = 0
	}
	if target > maxOffset {
		target = maxOffset
	}
	m.contentOffset = target
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

	lines := m.contentLines
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
			line = lines[idx]
			if m.matchNode == m.selected && m.matchQuery != "" && m.matchIndex >= 0 {
				line = highlightMatchedLine(lines, idx, m.matchQuery, m.matchIndex)
			}
			line = truncateANSI(line, textWidth)
		}
		line = padToWidthANSI(line, textWidth)
		if m.contentSelect {
			line = contentSelectionStyle.Width(textWidth).Render(line)
		}
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
	visibleHeight := m.treeVisibleDataRows()
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

func (m *Model) treeVisibleDataRows() int {
	_, _, paneHeight := m.layout()
	// Keep this in sync with View(): mainHeight = paneHeight - 1, treeInnerHeight = mainHeight - 2,
	// and one tree row is consumed by the table header.
	return paneHeight - 4
}

func (m *Model) scrollContent(delta int) {
	lines := m.contentLines
	if len(lines) == 0 {
		m.contentOffset = 0
		return
	}
	m.contentSelect = false
	contentInnerHeight := m.contentInnerHeight()
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

func copyToClipboard(text string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "windows":
		cmd = exec.Command("cmd", "/c", "clip")
	default:
		if _, err := exec.LookPath("wl-copy"); err == nil {
			cmd = exec.Command("wl-copy")
		} else if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		}
	}
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

func (m *Model) adjustContentOffset() {
	lines := m.contentLines
	contentInnerHeight := m.contentInnerHeight()
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

func (m Model) aclInnerHeight(width, mainHeight int) int {
	lines := m.renderACLLines(width, math.MaxInt)
	desired := len(lines)
	if desired < 1 {
		desired = 1
	}
	// Keep at least one content row visible below metadata+ACL sections.
	maxACL := mainHeight - metadataInnerHeight - 6 - 1
	if maxACL < 1 {
		maxACL = 1
	}
	if desired > maxACL {
		desired = maxACL
	}
	return desired
}

func (m Model) contentInnerHeight() int {
	_, rightOuter, paneHeight := m.layout()
	mainHeight := paneHeight - 1
	if mainHeight < 6 {
		mainHeight = 6
	}
	rightInner := rightOuter - 2
	aclInner := m.aclInnerHeight(rightInner, mainHeight)
	contentInnerHeight := mainHeight - metadataInnerHeight - aclInner - 6
	if contentInnerHeight < 1 {
		contentInnerHeight = 1
	}
	return contentInnerHeight
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
	items := []string{
		statusKeyStyle.Render("^Q") + " Quit",
		statusKeyStyle.Render("^S") + " Show stats",
		statusKeyStyle.Render("^F") + " Search",
		statusKeyStyle.Render("Tab") + " Switch panels",
		statusKeyStyle.Render("^O") + " Change sort order",
		statusKeyStyle.Render("^R") + " Reverse sort order",
	}
	if m.focus == focusContent {
		items = append(items,
			statusKeyStyle.Render("^A")+" Select all",
			statusKeyStyle.Render("^C")+" Copy",
		)
	}
	text := strings.Join(items, " | ")
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

func (m Model) renderSearchDialog(totalWidth int) string {
	title := "Search nodes (name + content)"
	if m.searchScope == searchContent {
		title = "Search content"
	}
	dialogWidth := totalWidth - 10
	if dialogWidth < 40 {
		dialogWidth = 40
	}
	if dialogWidth > 90 {
		dialogWidth = 90
	}
	cursor := "█"
	inputPrefix := "Query: "
	fieldWidth := dialogWidth - 6 - lipgloss.Width(inputPrefix)
	if fieldWidth < 8 {
		fieldWidth = 8
	}
	fieldText := rightCropToWidth(m.searchInput+cursor, fieldWidth)
	inputLine := inputPrefix + searchInputStyle.Width(fieldWidth).Render(fieldText)
	lines := []string{
		title,
		"",
		inputLine,
		"",
		"Enter = find next | Esc = cancel",
	}
	if m.searchRunning {
		spin := []string{"-", "\\", "|", "/"}[m.searchSpinStep%4]
		lines = append(lines, "Searching... "+spin)
	} else if m.searchMessage != "" {
		lines = append(lines, m.searchMessage)
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("39")).
		Padding(1, 2).
		Width(dialogWidth).
		Render(strings.Join(lines, "\n"))
}

func highlightMatchedLine(lines []string, lineIdx int, query string, matchIndex int) string {
	if lineIdx < 0 || lineIdx >= len(lines) || query == "" || matchIndex < 0 {
		return lines[lineIdx]
	}
	line := ansiEscapeRE.ReplaceAllString(lines[lineIdx], "")
	lineStart := 0
	for i := 0; i < lineIdx; i++ {
		lineStart += len(ansiEscapeRE.ReplaceAllString(lines[i], "")) + 1
	}
	lineEnd := lineStart + len(line)
	matchEnd := matchIndex + len(query)
	if matchEnd <= lineStart || matchIndex >= lineEnd {
		return line
	}
	relStart := matchIndex - lineStart
	if relStart < 0 {
		relStart = 0
	}
	relEnd := matchEnd - lineStart
	if relEnd > len(line) {
		relEnd = len(line)
	}
	if relStart >= relEnd {
		return line
	}
	return line[:relStart] + searchMatchStyle.Render(line[relStart:relEnd]) + line[relEnd:]
}

func rightCropToWidth(s string, max int) string {
	if max <= 0 || s == "" {
		return ""
	}
	if lipgloss.Width(s) <= max {
		return s
	}
	runes := []rune(s)
	start := len(runes)
	width := 0
	for start > 0 {
		r := runes[start-1]
		rw := lipgloss.Width(string(r))
		if width+rw > max {
			break
		}
		start--
		width += rw
	}
	return string(runes[start:])
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
