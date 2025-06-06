// Copyright 2018 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pkg

import (
	"bytes"
	"context"
	"io"
	"os"
	"sync"

	"go.chromium.org/luci/common/errors"
)

// Source is an underlying data source with CIPD package data.
//
// All errors are assumed to be IO errors and may be returned unannotated.
// Higher layers of the CIPD client will annotated them.
type Source interface {
	io.ReaderAt

	// Size returns the package file size.
	Size() int64

	// Close can be used to indicate to the storage (filesystem and/or cache)
	// layer that this source is actually bad. The storage layer can then evict
	// the bad file.
	//
	// The context is used for logging.
	Close(ctx context.Context, corrupt bool) error
}

// FileSource implements Source on top of a file system file.
type FileSource struct {
	file *os.File
	size int64
}

func (fs *FileSource) ReadAt(p []byte, off int64) (n int, err error) { return fs.file.ReadAt(p, off) }
func (fs *FileSource) Size() int64                                   { return fs.size }
func (fs *FileSource) Close(ctx context.Context, corrupt bool) error { return fs.file.Close() }

// NewFileSource opens a file for reading and returns it as a *FileSource.
func NewFileSource(path string) (*FileSource, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	stat, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	return &FileSource{
		file: f,
		size: stat.Size(),
	}, nil
}

// BytesSource implements Source on top of a byte buffer.
type BytesSource struct {
	r *io.SectionReader
}

func (bs *BytesSource) ReadAt(p []byte, off int64) (n int, err error) { return bs.r.ReadAt(p, off) }
func (bs *BytesSource) Size() int64                                   { return bs.r.Size() }
func (bs *BytesSource) Close(ctx context.Context, corrupt bool) error { return nil }

// NewBytesSource returns a Source implemented on top of a byte buffer.
func NewBytesSource(b []byte) *BytesSource {
	return &BytesSource{r: io.NewSectionReader(bytes.NewReader(b), 0, int64(len(b)))}
}

// ReadSeekerSource implements Source on top of an io.ReadSeeker.
//
// Useful for compatibility with the previous CIPD client API.
//
// All reads will be serialized. To gain any benefits from parallelization,
// switch to using Source interface directly instead of using io.ReadSeeker.
type ReadSeekerSource struct {
	m    sync.Mutex
	r    io.ReadSeeker
	ptr  int64
	size int64
}

func (r *ReadSeekerSource) ReadAt(data []byte, off int64) (int, error) {
	if off < 0 {
		return 0, errors.New("ReadSeekerSource.ReadAt: negative offset")
	}

	// Make sure we don't exceed the advertised size even if the underlying
	// io.Reader grew somehow.
	if off >= r.size {
		return 0, io.EOF
	}
	if int64(len(data)) > r.size-off {
		data = data[0 : r.size-off]
	}

	r.m.Lock()
	defer r.m.Unlock()

	// Skip Seek operation if happen to read the file sequentially.
	if off != r.ptr {
		if _, err := r.r.Seek(off, io.SeekStart); err != nil {
			return 0, err
		}
		r.ptr = off
	}

	n, err := r.r.Read(data)
	r.ptr += int64(n)
	return n, err
}

func (r *ReadSeekerSource) Size() int64                                   { return r.size }
func (r *ReadSeekerSource) Close(ctx context.Context, corrupt bool) error { return nil }

// NewReadSeekerSource returns a Source implemented on top of an io.ReadSeeker.
func NewReadSeekerSource(r io.ReadSeeker) (*ReadSeekerSource, error) {
	off, err := r.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, err
	}
	return &ReadSeekerSource{
		r:    r,
		ptr:  off, // just seeked there
		size: off,
	}, nil
}
