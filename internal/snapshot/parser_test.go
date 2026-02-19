package snapshot

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestParseFileBuildsTree(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "snapshot.test")
	if err := os.WriteFile(tmp, buildTestSnapshot(), 0o644); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}

	tree, err := ParseFile(tmp)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	if tree.Root == nil {
		t.Fatal("expected root node")
	}
	if tree.Root.ID != "/" {
		t.Fatalf("unexpected root ID: %q", tree.Root.ID)
	}

	a := tree.NodesByPath["/a"]
	if a == nil {
		t.Fatal("expected /a node")
	}
	if a.Parent != tree.Root {
		t.Fatal("expected /a parent root")
	}
	if string(a.Data) != `{"k":1}` {
		t.Fatalf("unexpected /a data: %q", string(a.Data))
	}

	b := tree.NodesByPath["/a/b"]
	if b == nil {
		t.Fatal("expected /a/b node")
	}
	if b.Parent != a {
		t.Fatal("expected /a/b parent /a")
	}
	if len(tree.ACLs[1]) != 1 {
		t.Fatalf("expected ACL ref 1 with one entry, got %d", len(tree.ACLs[1]))
	}
	if tree.ACLs[1][0].Scheme != "world" || tree.ACLs[1][0].ID != "anyone" {
		t.Fatalf("unexpected ACL entry: %+v", tree.ACLs[1][0])
	}
}

func TestParseFileRejectsBadMagic(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "snapshot.bad")
	b := buildTestSnapshot()
	b[0] = 0x00
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}

	if _, err := ParseFile(tmp); err == nil {
		t.Fatal("expected parse error for bad magic")
	}
}

func buildTestSnapshot() []byte {
	var b bytes.Buffer

	writeI32(&b, snapshotMagic) // magic
	writeI32(&b, 2)             // version
	writeI64(&b, -1)            // dbid

	// sessions
	writeI32(&b, 1)
	writeI64(&b, 42)
	writeI32(&b, 30000)

	// ACL map
	writeI32(&b, 1) // map size
	writeI64(&b, 1) // ACL ref
	writeI32(&b, 1) // ACL list length
	writeI32(&b, 31)
	writeString(&b, "world")
	writeString(&b, "anyone")

	// nodes in pre-order
	writeNode(&b, "", nil, -1)
	writeNode(&b, "/a", []byte(`{"k":1}`), 1)
	writeNode(&b, "/a/b", []byte("child"), 1)
	writeNode(&b, "/c", []byte("plain"), -1)

	// end marker
	writeString(&b, "/")

	// seal
	writeI64(&b, 0)
	writeString(&b, "/")

	return b.Bytes()
}

func writeNode(b *bytes.Buffer, path string, data []byte, aclRef int64) {
	writeString(b, path)
	writeBuffer(b, data)
	writeI64(b, aclRef)

	// stat persisted
	writeI64(b, 1)
	writeI64(b, 2)
	writeI64(b, 3)
	writeI64(b, 4)
	writeI32(b, 5)
	writeI32(b, 6)
	writeI32(b, 7)
	writeI64(b, 8)
	writeI64(b, 9)
}

func writeString(b *bytes.Buffer, s string) {
	writeI32(b, int32(len(s)))
	b.WriteString(s)
}

func writeBuffer(b *bytes.Buffer, v []byte) {
	if v == nil {
		writeI32(b, -1)
		return
	}
	writeI32(b, int32(len(v)))
	b.Write(v)
}

func writeI32(b *bytes.Buffer, v int32) {
	_ = binary.Write(b, binary.BigEndian, v)
}

func writeI64(b *bytes.Buffer, v int64) {
	_ = binary.Write(b, binary.BigEndian, v)
}
