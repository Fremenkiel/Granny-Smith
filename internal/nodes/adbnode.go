package nodes

import (
	"context"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

type AdbNode struct {
	fs.Inode
}

// Node types must be InodeEmbedders
var _ = (fs.InodeEmbedder)((*AdbNode)(nil))

// Node types should implement some file system operations, eg. Lookup
var _ = (fs.NodeLookuper)((*AdbNode)(nil))

func (n *AdbNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	ops := AdbNode{}
	out.Mode = 0755
	out.Size = 42
	return n.NewInode(ctx, &ops, fs.StableAttr{Mode: syscall.S_IFREG}), 0
}
