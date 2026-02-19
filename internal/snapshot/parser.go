package snapshot

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	snapshotMagic = 0x5A4B534E // "ZKSN"
	maxStringLen  = int32(16 * 1024 * 1024)
	maxBufferLen  = int32(256 * 1024 * 1024)
)

type Header struct {
	Magic   int32
	Version int32
	DBID    int64
}

type StatPersisted struct {
	Czxid          int64
	Mzxid          int64
	Ctime          int64
	Mtime          int64
	Version        int32
	Cversion       int32
	Aversion       int32
	EphemeralOwner int64
	Pzxid          int64
}

type Node struct {
	ID       string
	Path     string
	Data     []byte
	ACLRef   int64
	Stat     StatPersisted
	Parent   *Node
	Children []*Node
}

type ACL struct {
	Perms  int32
	Scheme string
	ID     string
}

type Tree struct {
	Header      Header
	Root        *Node
	NodesByPath map[string]*Node
	ACLs        map[int64][]ACL
}

func ParseFile(path string) (*Tree, error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("open snapshot file: %w", err)
	}
	defer f.Close()

	d := newDecoder(f)
	header, err := parseHeader(d)
	if err != nil {
		return nil, err
	}

	if err := parseSessions(d); err != nil {
		return nil, err
	}
	acls, err := parseACLCache(d)
	if err != nil {
		return nil, err
	}

	tree, err := parseNodes(d, header, acls)
	if err != nil {
		return nil, err
	}

	// Read and ignore the first seal (checksum + "/"), if present.
	if _, err := d.ReadInt64(); err == nil {
		if _, err := d.ReadString(maxStringLen); err != nil {
			return nil, err
		}
	}

	return tree, nil
}

func parseHeader(d *decoder) (Header, error) {
	magic, err := d.ReadInt32()
	if err != nil {
		return Header{}, err
	}
	version, err := d.ReadInt32()
	if err != nil {
		return Header{}, err
	}
	dbid, err := d.ReadInt64()
	if err != nil {
		return Header{}, err
	}
	if magic != snapshotMagic {
		return Header{}, fmt.Errorf("invalid snapshot magic %x", magic)
	}
	return Header{Magic: magic, Version: version, DBID: dbid}, nil
}

func parseSessions(d *decoder) error {
	count, err := d.ReadInt32()
	if err != nil {
		return err
	}
	if count < 0 {
		return fmt.Errorf("invalid session count %d", count)
	}
	for i := int32(0); i < count; i++ {
		if _, err := d.ReadInt64(); err != nil {
			return err
		}
		if _, err := d.ReadInt32(); err != nil {
			return err
		}
	}
	return nil
}

func parseACLCache(d *decoder) (map[int64][]ACL, error) {
	count, err := d.ReadInt32()
	if err != nil {
		return nil, err
	}
	if count < 0 {
		return nil, fmt.Errorf("invalid ACL map size %d", count)
	}
	acls := make(map[int64][]ACL, count)
	for i := int32(0); i < count; i++ {
		ref, err := d.ReadInt64()
		if err != nil {
			return nil, err
		}
		vecLen, err := d.ReadInt32()
		if err != nil {
			return nil, err
		}
		if vecLen < 0 {
			return nil, fmt.Errorf("invalid ACL vector length %d", vecLen)
		}
		list := make([]ACL, 0, vecLen)
		for j := int32(0); j < vecLen; j++ {
			perms, err := d.ReadInt32()
			if err != nil {
				return nil, err
			}
			scheme, err := d.ReadString(maxStringLen)
			if err != nil {
				return nil, err
			}
			id, err := d.ReadString(maxStringLen)
			if err != nil {
				return nil, err
			}
			list = append(list, ACL{Perms: perms, Scheme: scheme, ID: id})
		}
		acls[ref] = list
	}
	return acls, nil
}

func parseNodes(d *decoder, header Header, acls map[int64][]ACL) (*Tree, error) {
	nodes := make(map[string]*Node)

	for {
		path, err := d.ReadString(maxStringLen)
		if err != nil {
			return nil, err
		}
		if path == "/" {
			break
		}

		data, err := d.ReadBuffer(maxBufferLen)
		if err != nil {
			return nil, err
		}
		aclRef, err := d.ReadInt64()
		if err != nil {
			return nil, err
		}

		stat, err := parseStatPersisted(d)
		if err != nil {
			return nil, err
		}

		node := &Node{
			ID:     nodeID(path),
			Path:   path,
			Data:   data,
			ACLRef: aclRef,
			Stat:   stat,
		}
		nodes[path] = node

		if path == "" {
			continue
		}

		parentPath := parentOf(path)
		parent, ok := nodes[parentPath]
		if !ok {
			return nil, fmt.Errorf("invalid tree: parent %q for path %q not found", parentPath, path)
		}
		node.Parent = parent
		parent.Children = append(parent.Children, node)
	}

	root, ok := nodes[""]
	if !ok {
		return nil, fmt.Errorf("invalid snapshot: missing root node")
	}

	// Mirror ZooKeeper behavior where "/" also points to root.
	nodes["/"] = root

	return &Tree{
		Header:      header,
		Root:        root,
		NodesByPath: nodes,
		ACLs:        acls,
	}, nil
}

func parseStatPersisted(d *decoder) (StatPersisted, error) {
	czxid, err := d.ReadInt64()
	if err != nil {
		return StatPersisted{}, err
	}
	mzxid, err := d.ReadInt64()
	if err != nil {
		return StatPersisted{}, err
	}
	ctime, err := d.ReadInt64()
	if err != nil {
		return StatPersisted{}, err
	}
	mtime, err := d.ReadInt64()
	if err != nil {
		return StatPersisted{}, err
	}
	version, err := d.ReadInt32()
	if err != nil {
		return StatPersisted{}, err
	}
	cversion, err := d.ReadInt32()
	if err != nil {
		return StatPersisted{}, err
	}
	aversion, err := d.ReadInt32()
	if err != nil {
		return StatPersisted{}, err
	}
	ephemeralOwner, err := d.ReadInt64()
	if err != nil {
		return StatPersisted{}, err
	}
	pzxid, err := d.ReadInt64()
	if err != nil {
		return StatPersisted{}, err
	}
	return StatPersisted{
		Czxid:          czxid,
		Mzxid:          mzxid,
		Ctime:          ctime,
		Mtime:          mtime,
		Version:        version,
		Cversion:       cversion,
		Aversion:       aversion,
		EphemeralOwner: ephemeralOwner,
		Pzxid:          pzxid,
	}, nil
}

func parentOf(path string) string {
	idx := strings.LastIndex(path, "/")
	if idx <= 0 {
		return ""
	}
	return path[:idx]
}

func nodeID(path string) string {
	if path == "" || path == "/" {
		return "/"
	}
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}
