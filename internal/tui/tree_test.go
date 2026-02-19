package tui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/jowiho/zooxplorer/internal/snapshot"
)

func TestFlattenAndRenderTree(t *testing.T) {
	root := &snapshot.Node{ID: "/", Path: ""}
	b := &snapshot.Node{ID: "b", Path: "/b", Parent: root, Data: []byte("bbb")}
	a := &snapshot.Node{ID: "a", Path: "/a", Parent: root, Data: []byte("aaaa")}
	a1 := &snapshot.Node{ID: "a1", Path: "/a/a1", Parent: a, Data: []byte("cc")}
	root.Children = []*snapshot.Node{b, a}
	a.Children = []*snapshot.Node{a1}

	rows := flatten(root, map[string]bool{})
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].Node != a || rows[0].Depth != 0 {
		t.Fatalf("unexpected row at index 0: %#v", rows[0])
	}

	rows = flatten(root, map[string]bool{"/a": true})
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows after expand, got %d", len(rows))
	}
	if rows[1].Node != a1 || rows[1].Depth != 1 {
		t.Fatalf("unexpected row at index 1: %#v", rows[1])
	}

	view := renderTree(rows, a1, 80, map[string]bool{"/a": true})
	view = stripANSI(view)
	if !strings.Contains(view, "- a") {
		t.Fatalf("expected expanded marker in view:\n%s", view)
	}
	if !strings.Contains(view, ">     a1") {
		t.Fatalf("expected selected indicator in view:\n%s", view)
	}
	if !strings.Contains(view, "a [size=4 total=6 children=1]") {
		t.Fatalf("expected parent size metadata in view:\n%s", view)
	}
	if !strings.Contains(view, "a1 [size=2]") {
		t.Fatalf("expected leaf size metadata in view:\n%s", view)
	}
}

func stripANSI(s string) string {
	re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return re.ReplaceAllString(s, "")
}
