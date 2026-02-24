package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jowiho/zooxplorer/internal/snapshot"
)

func TestFormatSnapshotTimeUTC(t *testing.T) {
	got := formatSnapshotTimeUTC(0)
	if got != "1970-01-01T00:00:00Z" {
		t.Fatalf("unexpected time format: %q", got)
	}
}

func TestModelQuitKeys(t *testing.T) {
	m := NewModel(sampleSnapshotTree())

	tests := []tea.KeyMsg{
		{Type: tea.KeyCtrlC},
	}

	for _, key := range tests {
		_, cmd := m.Update(key)
		if cmd == nil {
			t.Fatal("expected quit command")
		}
	}
}

func TestModelArrowNavigation(t *testing.T) {
	m := NewModel(sampleSnapshotTree())

	var cmd tea.Cmd
	var model tea.Model = m
	typed := model.(Model)
	if typed.selected.Path != "/a" {
		t.Fatalf("expected initial /a selected, got %q", typed.selected.Path)
	}

	model, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRight})
	if cmd != nil {
		t.Fatal("unexpected command")
	}
	typed = model.(Model)
	if !typed.expanded["/a"] {
		t.Fatal("expected /a expanded")
	}
	if len(typed.rows) != 3 {
		t.Fatalf("expected 3 visible rows, got %d", len(typed.rows))
	}

	model, cmd = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	if cmd != nil {
		t.Fatal("unexpected command")
	}
	typed = model.(Model)
	if typed.selected.Path != "/a/a1" {
		t.Fatalf("expected /a/a1 selected, got %q", typed.selected.Path)
	}

	model, cmd = model.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if cmd != nil {
		t.Fatal("unexpected command")
	}
	typed = model.(Model)
	if len(typed.rows) != 3 {
		t.Fatalf("expected leaf collapse to keep 3 rows, got %d", len(typed.rows))
	}

	// Move to parent and collapse it.
	model, cmd = model.Update(tea.KeyMsg{Type: tea.KeyUp})
	if cmd != nil {
		t.Fatal("unexpected command")
	}
	typed = model.(Model)
	if typed.selected.Path != "/a" {
		t.Fatalf("expected /a selected, got %q", typed.selected.Path)
	}

	model, cmd = model.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if cmd != nil {
		t.Fatal("unexpected command")
	}
	typed = model.(Model)
	if typed.expanded["/a"] {
		t.Fatal("expected /a collapsed")
	}
	if len(typed.rows) != 2 {
		t.Fatalf("expected 2 visible rows, got %d", len(typed.rows))
	}

	model, cmd = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	if cmd != nil {
		t.Fatal("unexpected command")
	}
	typed = model.(Model)
	if typed.selected.Path != "/b" {
		t.Fatalf("expected /b selected, got %q", typed.selected.Path)
	}

	model, _ = typed.Update(tea.KeyMsg{Type: tea.KeyUp})
	typed = model.(Model)
	if typed.selected.Path != "/a" {
		t.Fatalf("expected /a selected, got %q", typed.selected.Path)
	}
}

func TestModelTreeScrollOffsetTracksSelection(t *testing.T) {
	root := &snapshot.Node{ID: "/", Path: ""}
	nodes := make([]*snapshot.Node, 0, 10)
	for i := 0; i < 10; i++ {
		n := &snapshot.Node{ID: string(rune('a' + i)), Path: "/n" + string(rune('a'+i)), Parent: root}
		nodes = append(nodes, n)
	}
	root.Children = nodes

	model := NewModel(&snapshot.Tree{
		Root: root,
	})

	var m tea.Model = model
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 8})
	for i := 0; i < 6; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}

	typed := m.(Model)
	if typed.selected != nodes[6] {
		t.Fatalf("expected selected node index 6")
	}
	if typed.treeOffset == 0 {
		t.Fatalf("expected tree offset to scroll, got %d", typed.treeOffset)
	}
}

func TestTreeScrollDownFromBottomVisibleRow(t *testing.T) {
	root := &snapshot.Node{ID: "/", Path: ""}
	nodes := make([]*snapshot.Node, 0, 8)
	for i := 0; i < 8; i++ {
		n := &snapshot.Node{ID: string(rune('a' + i)), Path: "/n" + string(rune('a'+i)), Parent: root}
		nodes = append(nodes, n)
	}
	root.Children = nodes

	model := NewModel(&snapshot.Tree{Root: root})
	var m tea.Model = model
	// With height=8, visible tree data rows should be 4 (after status bar, borders, and table header).
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 8})

	for i := 0; i < 3; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	typed := m.(Model)
	if typed.selected != nodes[3] {
		t.Fatalf("expected selection at last visible row node, got %q", typed.selected.Path)
	}
	if typed.treeOffset != 0 {
		t.Fatalf("expected offset 0 before overflow move, got %d", typed.treeOffset)
	}

	// Moving once more should select next node and scroll it into view.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	typed = m.(Model)
	if typed.selected != nodes[4] {
		t.Fatalf("expected selection at next node, got %q", typed.selected.Path)
	}
	if typed.treeOffset != 1 {
		t.Fatalf("expected offset 1 after overflow move, got %d", typed.treeOffset)
	}
}

func TestModelTabSwitchesFocusAndScrollsContent(t *testing.T) {
	model := NewModel(sampleSnapshotTree())
	var m tea.Model = model
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 10})

	typed := m.(Model)
	if typed.focus != focusTree {
		t.Fatalf("expected initial focus on tree")
	}
	selectedPath := typed.selected.Path

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	typed = m.(Model)
	if typed.focus != focusContent {
		t.Fatalf("expected focus on content after tab")
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	typed = m.(Model)
	if typed.selected.Path != selectedPath {
		t.Fatalf("expected selected node unchanged while scrolling content")
	}
	if typed.contentOffset == 0 {
		t.Fatalf("expected content to scroll down")
	}
}

func TestRenderMetadataIncludesSize(t *testing.T) {
	m := NewModel(sampleSnapshotTree())
	meta := m.renderMetadata()
	if !strings.Contains(meta, "/a ID 1 (version 5)") {
		t.Fatalf("expected first metadata line with path/id/version, got: %q", meta)
	}
	if !strings.Contains(meta, "Size: ") {
		t.Fatalf("expected metadata to include size, got: %q", meta)
	}
	if strings.Contains(meta, "acl=") || strings.Contains(meta, "acl_version=") {
		t.Fatalf("metadata should not include ACL fields anymore, got: %q", meta)
	}
}

func TestModelCtrlOCyclesSortColumn(t *testing.T) {
	m := NewModel(sampleSnapshotTree())
	var model tea.Model = m

	expected := []sortColumn{
		sortByNodeSize,
		sortBySubtreeSize,
		sortByChildren,
		sortByModified,
		sortByNodeName,
	}
	for _, want := range expected {
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
		got := model.(Model).sortOrder
		if got != want {
			t.Fatalf("unexpected sort column, got=%v want=%v", got, want)
		}
	}
}

func TestModelCtrlRReversesCurrentSortOrder(t *testing.T) {
	m := NewModel(sampleSnapshotTree())
	var model tea.Model = m

	typed := model.(Model)
	if typed.sortDesc[sortByNodeName] {
		t.Fatal("expected node-name sort ascending by default")
	}

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	typed = model.(Model)
	if !typed.sortDesc[sortByNodeName] {
		t.Fatal("expected node-name sort reversed to descending")
	}

	// Switch to node-size (default descending) and reverse.
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	typed = model.(Model)
	if !typed.sortDesc[sortByNodeSize] {
		t.Fatal("expected node-size sort descending by default")
	}
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	typed = model.(Model)
	if typed.sortDesc[sortByNodeSize] {
		t.Fatal("expected node-size sort reversed to ascending")
	}
}

func TestModelAltUpJumpsToParent(t *testing.T) {
	m := NewModel(sampleSnapshotTree())
	var model tea.Model = m

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRight})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	typed := model.(Model)
	if typed.selected.Path != "/a/a1" {
		t.Fatalf("expected /a/a1 selected, got %q", typed.selected.Path)
	}

	model, _ = typed.Update(tea.KeyMsg{Type: tea.KeyUp, Alt: true})
	typed = model.(Model)
	if typed.selected.Path != "/a" {
		t.Fatalf("expected parent /a selected, got %q", typed.selected.Path)
	}

	model, _ = typed.Update(tea.KeyMsg{Type: tea.KeyUp, Alt: true})
	typed = model.(Model)
	if typed.selected.Path != "/a" {
		t.Fatalf("expected top-level selection to remain /a, got %q", typed.selected.Path)
	}
}

func TestRenderACLIncludesDigestUsernameAndPermissions(t *testing.T) {
	m := NewModel(sampleSnapshotTree())
	acl := m.renderACL()
	if !strings.Contains(acl, "ACL ID 1 (version 7)") {
		t.Fatalf("expected ACL ID/version, got: %q", acl)
	}
	if !strings.Contains(acl, "alice: create|read|write") {
		t.Fatalf("expected digest ACL entry format, got: %q", acl)
	}
	if strings.Contains(acl, "secret") {
		t.Fatalf("digest password/hash should not be displayed, got: %q", acl)
	}
	if !strings.Contains(acl, "perms=all") {
		t.Fatalf("expected non-digest entries to still show perms label, got: %q", acl)
	}
}

func TestModelCtrlSShowsStatsAndAnyKeyCloses(t *testing.T) {
	m := NewModel(sampleSnapshotTree())
	var model tea.Model = m
	selectedPath := m.selected.Path

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	typed := model.(Model)
	if !typed.statsOpen {
		t.Fatal("expected stats dialog to be open")
	}
	stats := typed.statsText
	if !strings.Contains(stats, "Snapshot Statistics") {
		t.Fatalf("expected stats header, got: %q", stats)
	}
	if !strings.Contains(stats, "Total nodes    : 4") {
		t.Fatalf("expected total node count, got: %q", stats)
	}
	if !strings.Contains(stats, "Ephemeral nodes: 1") {
		t.Fatalf("expected ephemeral count, got: %q", stats)
	}
	if !strings.Contains(stats, "Empty nodes    : 3") {
		t.Fatalf("expected empty count, got: %q", stats)
	}
	if !strings.Contains(stats, "Biggest node:") || !strings.Contains(stats, "/a") {
		t.Fatalf("expected biggest node details, got: %q", stats)
	}

	// Any key closes the dialog and should not trigger normal key handling.
	model, _ = typed.Update(tea.KeyMsg{Type: tea.KeyDown})
	typed = model.(Model)
	if typed.statsOpen {
		t.Fatal("expected stats dialog to be closed")
	}
	if typed.selected.Path != selectedPath {
		t.Fatalf("expected selection unchanged when closing stats dialog, got %q", typed.selected.Path)
	}
}

func sampleSnapshotTree() *snapshot.Tree {
	root := &snapshot.Node{ID: "/", Path: ""}
	a := &snapshot.Node{
		ID:     "a",
		Path:   "/a",
		Parent: root,
		Data:   []byte("line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8"),
		ACLRef: 1,
		Stat:   snapshot.StatPersisted{Version: 5, Aversion: 7},
	}
	b := &snapshot.Node{
		ID:     "b",
		Path:   "/b",
		Parent: root,
		Stat:   snapshot.StatPersisted{EphemeralOwner: 42},
	}
	a1 := &snapshot.Node{ID: "a1", Path: "/a/a1", Parent: a}
	root.Children = []*snapshot.Node{a, b}
	a.Children = []*snapshot.Node{a1}

	return &snapshot.Tree{
		Root:        root,
		NodesByPath: map[string]*snapshot.Node{"": root, "/": root, "/a": a, "/a/a1": a1, "/b": b},
		ACLs: map[int64][]snapshot.ACL{
			1: []snapshot.ACL{
				{Perms: 7, Scheme: "digest", ID: "alice:secret"},
				{Perms: 31, Scheme: "world", ID: "anyone"},
			},
		},
	}
}
