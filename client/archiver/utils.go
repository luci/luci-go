// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package archiver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/luci/luci-go/common/isolated"
)

func newInt(v int) *int {
	o := new(int)
	*o = v
	return o
}

func newInt64(v int64) *int64 {
	o := new(int64)
	*o = v
	return o
}

func newString(v string) *string {
	o := new(string)
	*o = v
	return o
}

type simpleFuture struct {
	wgHashed sync.WaitGroup
	lock     sync.Mutex
	err      error
	digest   isolated.HexDigest
}

func (s *simpleFuture) WaitForHashed() {
	s.wgHashed.Wait()
}

func (s *simpleFuture) Error() error {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.err
}

func (s *simpleFuture) Digest() isolated.HexDigest {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.digest
}

type walkItem struct {
	fullPath string
	relPath  string
	info     os.FileInfo
	err      error
}

// walk() enumerates a directory tree synchronously and sends the items to
// channel c.
//
// blacklist is a list of globs of files to ignore.
func walk(root string, blacklist []string, c chan<- *walkItem) {
	// TODO(maruel): Walk() sorts the file names list, which is not needed here
	// and slows things down. Options:
	// #1 Use os.File.Readdir() directly. It's in the stdlib and works fine, but
	//    it's not the most efficient implementation. On posix it does a lstat()
	//    call, on Windows it does a Win32FileAttributeData.
	// #2 Use raw syscalls.
	//   - On POSIX, use syscall.ReadDirent(). See src/os/dir_unix.go.
	//   - On Windows, use syscall.FindFirstFile(), syscall.FindNextFile(),
	//     syscall.FindClose() directly. See src/os/file_windows.go. For odd
	//     reasons, Windows does not have a batched version to reduce the number
	//     of kernel calls. It's as if they didn't care about performance.
	//
	// In practice, #2 may not be needed, the performance of #1 may be good
	// enough relative to the other performance costs. This needs to be perf
	// tested at 100k+ files scale on Windows and OSX.
	//
	// TODO(maruel): Cache directory enumeration. In particular cases (Chromium),
	// the same directory may be enumerated multiple times. Caching the content
	// may be worth. This needs to be perf tested.

	// Check patterns upfront, so it has consistent behavior w.r.t. bad glob
	// patterns.
	for _, b := range blacklist {
		if _, err := filepath.Match(b, ""); err != nil {
			c <- &walkItem{err: fmt.Errorf("bad blacklist pattern \"%s\"", b)}
			return
		}
	}
	if strings.HasSuffix(root, string(filepath.Separator)) {
		root = root[:len(root)-1]
	}
	rootLen := len(root) + 1
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("walk(%s): %s", p, err)
		}
		if len(p) <= rootLen {
			// Root directory.
			return nil
		}
		relPath := p[rootLen:]
		for _, b := range blacklist {
			if matched, _ := filepath.Match(b, relPath); matched {
				// Must not return io.SkipDir for file, filepath.walk() handles this
				// badly.
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		if info.IsDir() {
			return nil
		}
		c <- &walkItem{fullPath: p, relPath: relPath, info: info}
		return nil
	})
	if err != nil {
		c <- &walkItem{err: err}
	}
}

// PushDirectory walks a directory at root and creates a .isolated file.
//
// It walks the directories synchronously, then hashes, does cache lookups and
// uploads asynchronously.
//
// blacklist is a list of globs of files to ignore.
func PushDirectory(a Archiver, root string, blacklist []string) Future {
	c := make(chan *walkItem)
	go func() {
		walk(root, blacklist, c)
		close(c)
	}()

	i := isolated.Isolated{
		Algo:    "sha-1",
		Files:   map[string]isolated.File{},
		Version: isolated.IsolatedFormatVersion,
	}
	futures := map[string]Future{}
	s := &simpleFuture{}
	for item := range c {
		if item.err != nil {
			s.err = item.err
		}
		if s.err != nil {
			// Empty the queue.
			continue
		}
		if filepath.Separator == '\\' {
			// Windows.
			item.relPath = strings.Replace(item.relPath, "\\", "/", -1)
		}
		mode := item.info.Mode()
		if mode&os.ModeSymlink == os.ModeSymlink {
			l, err := os.Readlink(item.fullPath)
			if err != nil {
				s.err = fmt.Errorf("readlink(%s): %s", item.fullPath, err)
			}
			i.Files[item.relPath] = isolated.File{Link: newString(l)}
		} else {
			i.Files[item.relPath] = isolated.File{
				Mode: newInt(int(mode.Perm())),
				Size: newInt64(item.info.Size()),
			}
			futures[item.relPath] = a.PushFile(item.fullPath)
		}
	}
	if s.err != nil {
		return s
	}
	log.Printf("PushDirectory(%s) = %d files", root, len(i.Files))

	// Hashing, cache lookups and upload is done asynchronously.
	s.wgHashed.Add(1)
	go func() {
		defer s.wgHashed.Done()
		var err error
		for p, future := range futures {
			future.WaitForHashed()
			if err = future.Error(); err != nil {
				break
			}
			d := i.Files[p]
			d.Digest = future.Digest()
			i.Files[p] = d
		}
		var d isolated.HexDigest
		if err == nil {
			raw := &bytes.Buffer{}
			if err = json.NewEncoder(raw).Encode(i); err == nil {
				f := a.Push(filepath.Base(root)+".isolated", bytes.NewReader(raw.Bytes()))
				f.WaitForHashed()
				err = f.Error()
				d = f.Digest()
			}
		}
		s.lock.Lock()
		s.err = err
		s.digest = d
		s.lock.Unlock()
	}()
	return s
}
