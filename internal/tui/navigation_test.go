package tui

import (
	"testing"

	"github.com/jowiho/zooxplorer/internal/snapshot"
)

func TestPreviousSiblingOrParent(t *testing.T) {
	root, a, b, c, b1 := sampleTree()

	tests := []struct {
		name string
		in   *snapshot.Node
		want *snapshot.Node
	}{
		{name: "root_stays_root", in: root, want: root},
		{name: "first_top_level_stays", in: a, want: a},
		{name: "middle_sibling_goes_prev", in: b, want: a},
		{name: "child_with_no_prev_goes_parent", in: b1, want: b},
		{name: "last_sibling_goes_prev", in: c, want: b},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := previousSiblingOrParent(tc.in); got != tc.want {
				t.Fatalf("got %q want %q", got.Path, tc.want.Path)
			}
		})
	}
}

func TestNextSiblingOrNextAncestor(t *testing.T) {
	root, a, b, c, b1 := sampleTree()

	tests := []struct {
		name string
		in   *snapshot.Node
		want *snapshot.Node
	}{
		{name: "root_no_next_stays", in: root, want: root},
		{name: "next_sibling", in: a, want: b},
		{name: "next_sibling_last_to_c", in: b, want: c},
		{name: "child_without_next_goes_parent_next_sibling", in: b1, want: c},
		{name: "last_node_stays", in: c, want: c},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := nextSiblingOrNextAncestor(tc.in); got != tc.want {
				t.Fatalf("got %q want %q", got.Path, tc.want.Path)
			}
		})
	}
}

func sampleTree() (root, a, b, c, b1 *snapshot.Node) {
	root = &snapshot.Node{ID: "/", Path: ""}
	a = &snapshot.Node{ID: "a", Path: "/a", Parent: root}
	b = &snapshot.Node{ID: "b", Path: "/b", Parent: root}
	c = &snapshot.Node{ID: "c", Path: "/c", Parent: root}
	b1 = &snapshot.Node{ID: "b1", Path: "/b/b1", Parent: b}

	root.Children = []*snapshot.Node{a, b, c}
	b.Children = []*snapshot.Node{b1}
	return root, a, b, c, b1
}
