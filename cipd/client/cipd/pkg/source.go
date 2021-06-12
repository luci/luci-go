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
)

// Source is an underlying data source with CIPD package data.
type Source interface {
	io.ReaderAt

	// Size returns the package file size.
	Size() int64

	// Close can be used to indicate to the storage (filesystem and/or cache)
	// layer that this source is actually bad. The storage layer can then evict
	// the bad file.
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
