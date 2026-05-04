package adbfs

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	adb "github.com/zach-klippenstein/goadb"
)

type storage struct {
	Device			*adb.Device
}

func newStorage(device *adb.Device) *storage {
	return &storage{
		Device: device,
	}
}

func (s *storage) Has(path string) bool {
	return true
}

func (s *storage) New(path string, mode os.FileMode, flag int) (*file, error) {
	return nil, nil
}

func (s *storage) createParent(path string, mode os.FileMode, f *file) error {
	return nil
}

func (s *storage) Children(path string) []*file {
	path = clean(path)

	l := make([]*file, 0)
	r, err := s.Device.ListDirEntries(path)
	if err != nil {
		return nil
	}

	lde, err := r.ReadAll()
	if err != nil {
		return nil
	}

	for _, e := range lde {
		l = append(l, newFile(e))
	}

	return l
}

func (s *storage) MustGet(path string) *file {
	f, ok := s.Get(path)
	if !ok {
		panic(fmt.Errorf("couldn't find %q", path))
	}

	return f
}

func (s *storage) Get(path string) (*file, bool) {
	path = clean(path)
	f, err := s.Device.Stat(path)
	if err != nil {
		return nil, false
	}

	if !f.Mode.IsDir() {
		return nil, false
	}

	return newFile(f), true
}

func (s *storage) Rename(from, to string) error {
	from = clean(from)
	to = clean(to)

	return nil
}

func (s *storage) move(from, to string) error {
	return nil
}

func (s *storage) Remove(path string) error {
	path = clean(path)

	/*
	f, has := s.Get(path)
	if !has {
		return os.ErrNotExist
	}
	*/

	return nil
}

func clean(path string) string {
	return filepath.Clean(filepath.FromSlash(path))
}

type content struct {
	name  string
	bytes []byte

	m sync.RWMutex
}

func (c *content) WriteAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, &os.PathError{
			Op:   "writeat",
			Path: c.name,
			Err:  errors.New("negative offset"),
		}
	}

	c.m.Lock()
	prev := len(c.bytes)

	diff := int(off) - prev
	if diff > 0 {
		c.bytes = append(c.bytes, make([]byte, diff)...)
	}

	c.bytes = append(c.bytes[:off], p...)
	if len(c.bytes) < prev {
		c.bytes = c.bytes[:prev]
	}
	c.m.Unlock()

	return len(p), nil
}

func (c *content) ReadAt(b []byte, off int64) (n int, err error) {
	if off < 0 {
		return 0, &os.PathError{
			Op:   "readat",
			Path: c.name,
			Err:  errors.New("negative offset"),
		}
	}

	c.m.RLock()
	size := int64(len(c.bytes))
	if off >= size {
		c.m.RUnlock()
		return 0, io.EOF
	}

	l := int64(len(b))
	if off+l > size {
		l = size - off
	}

	btr := c.bytes[off : off+l]
	n = copy(b, btr)

	if len(btr) < len(b) {
		err = io.EOF
	}
	c.m.RUnlock()

	return
}

func newFile(f *adb.DirEntry) *file {
	return &file{
		name: f.Name,
		mode: f.Mode,
		content: &content{name: f.Name},
		mtime: f.ModifiedAt,
	} 
}
