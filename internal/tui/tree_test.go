package tui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/jowiho/zooxplorer/internal/snapshot"
)

func TestFlattenAndRenderTree(t *testing.T) {
	root := &snapshot.Node{ID: "/", Path: ""}
	b := &snapshot.Node{ID: "b", Path: "/b", Parent: root, Data: []byte("bbb"), Stat: snapshot.StatPersisted{Mtime: 0}}
	a := &snapshot.Node{ID: "a", Path: "/a", Parent: root, Data: []byte("aaaa"), Stat: snapshot.StatPersisted{Mtime: 1000}}
	a1 := &snapshot.Node{ID: "a1", Path: "/a/a1", Parent: a, Data: []byte("cc"), Stat: snapshot.StatPersisted{Mtime: 2000}}
	root.Children = []*snapshot.Node{b, a}
	a.Children = []*snapshot.Node{a1}

	rows := flatten(root, map[string]bool{}, sortByNodeName, false)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].Node != a || rows[0].Depth != 0 {
		t.Fatalf("unexpected row at index 0: %#v", rows[0])
	}

	rows = flatten(root, map[string]bool{"/a": true}, sortByNodeName, false)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows after expand, got %d", len(rows))
	}
	if rows[1].Node != a1 || rows[1].Depth != 1 {
		t.Fatalf("unexpected row at index 1: %#v", rows[1])
	}

	view := renderTree(rows, a1, 80, map[string]bool{"/a": true}, sortByNodeName, false)
	view = stripANSI(view)
	if !strings.Contains(view, "▲ Node name") || !strings.Contains(view, "Node size") || !strings.Contains(view, "Modified") {
		t.Fatalf("expected table header in view:\n%s", view)
	}
	if !strings.Contains(view, "- a") {
		t.Fatalf("expected expanded marker in view:\n%s", view)
	}
	if !strings.Contains(view, ">     a1") {
		t.Fatalf("expected selected indicator in view:\n%s", view)
	}
	if !strings.Contains(view, "- a") || !regexp.MustCompile(`\b4\s+6\s+1\b`).MatchString(view) || !strings.Contains(view, "1970-01-01T00:00:01Z") {
		t.Fatalf("expected parent row values in table:\n%s", view)
	}
	if !strings.Contains(view, "a1") || !regexp.MustCompile(`\b2\s+2\s+0\b`).MatchString(view) || !strings.Contains(view, "1970-01-01T00:00:02Z") {
		t.Fatalf("expected leaf row values in table:\n%s", view)
	}
}

func TestFlattenSortByNodeSize(t *testing.T) {
	root := &snapshot.Node{ID: "/", Path: ""}
	a := &snapshot.Node{ID: "a", Path: "/a", Parent: root, Data: []byte("aaaa")}
	b := &snapshot.Node{ID: "b", Path: "/b", Parent: root, Data: []byte("bb")}
	a1 := &snapshot.Node{ID: "a1", Path: "/a/a1", Parent: a, Data: []byte("ccccc")}
	root.Children = []*snapshot.Node{a, b}
	a.Children = []*snapshot.Node{a1}

	rows := flatten(root, map[string]bool{}, sortByNodeSize, true)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	// Flat global sort for node-size mode: a1(5), a(4), b(2).
	if rows[0].Node != a1 || rows[1].Node != a || rows[2].Node != b {
		t.Fatalf("unexpected sort order by node size: %s, %s, %s", rows[0].Node.ID, rows[1].Node.ID, rows[2].Node.ID)
	}
	if rows[0].Depth != 0 || rows[1].Depth != 0 || rows[2].Depth != 0 {
		t.Fatalf("expected flat rows at depth 0, got depths: %d,%d,%d", rows[0].Depth, rows[1].Depth, rows[2].Depth)
	}
}

func TestFlattenSortByModifiedIsGlobalAndFlat(t *testing.T) {
	root := &snapshot.Node{ID: "/", Path: ""}
	a := &snapshot.Node{ID: "a", Path: "/a", Parent: root, Stat: snapshot.StatPersisted{Mtime: 3000}}
	b := &snapshot.Node{ID: "b", Path: "/b", Parent: root, Stat: snapshot.StatPersisted{Mtime: 2000}}
	a1 := &snapshot.Node{ID: "a1", Path: "/a/a1", Parent: a, Stat: snapshot.StatPersisted{Mtime: 1000}}
	root.Children = []*snapshot.Node{a, b}
	a.Children = []*snapshot.Node{a1}

	rows := flatten(root, map[string]bool{}, sortByModified, false)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	// Modified mode sorts oldest first globally: a1(1000), b(2000), a(3000).
	if rows[0].Node != a1 || rows[1].Node != b || rows[2].Node != a {
		t.Fatalf("unexpected sort order by modified: %s, %s, %s", rows[0].Node.ID, rows[1].Node.ID, rows[2].Node.ID)
	}
	if rows[0].Depth != 0 || rows[1].Depth != 0 || rows[2].Depth != 0 {
		t.Fatalf("expected flat rows at depth 0, got depths: %d,%d,%d", rows[0].Depth, rows[1].Depth, rows[2].Depth)
	}
}

func stripANSI(s string) string {
	re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return re.ReplaceAllString(s, "")
}
