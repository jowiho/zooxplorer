package tui

import "github.com/jowiho/zooxplorer/internal/snapshot"

func previousSiblingOrParent(node *snapshot.Node) *snapshot.Node {
	if node == nil {
		return nil
	}
	parent := node.Parent
	if parent == nil {
		return node
	}

	idx := childIndex(parent, node)
	if idx <= 0 {
		// Keep top-level navigation within top-level rows when root is hidden.
		if parent.Parent == nil {
			return node
		}
		return parent
	}
	return parent.Children[idx-1]
}

func nextSiblingOrNextAncestor(node *snapshot.Node) *snapshot.Node {
	if node == nil {
		return nil
	}
	if sibling := nextSibling(node); sibling != nil {
		return sibling
	}
	for p := node.Parent; p != nil; p = p.Parent {
		if sibling := nextSibling(p); sibling != nil {
			return sibling
		}
	}
	return node
}

func parentNode(node *snapshot.Node) *snapshot.Node {
	if node == nil || node.Parent == nil {
		return node
	}
	return node.Parent
}

func visibleParentNode(node *snapshot.Node) *snapshot.Node {
	parent := parentNode(node)
	if parent == nil {
		return node
	}
	// Root is hidden in the tree view, keep top-level selections in place.
	if parent.Parent == nil {
		return node
	}
	return parent
}

func firstChild(node *snapshot.Node) *snapshot.Node {
	if node == nil || len(node.Children) == 0 {
		return node
	}
	return node.Children[0]
}

func nextSibling(node *snapshot.Node) *snapshot.Node {
	parent := node.Parent
	if parent == nil {
		return nil
	}
	idx := childIndex(parent, node)
	if idx == -1 || idx+1 >= len(parent.Children) {
		return nil
	}
	return parent.Children[idx+1]
}

func childIndex(parent, child *snapshot.Node) int {
	for i := range parent.Children {
		if parent.Children[i] == child {
			return i
		}
	}
	return -1
}
