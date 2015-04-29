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

// PushDirectory walks a directory at root and creates a .isolated file.
//
// It walks the directories synchronously, then hashes, does cache lookups and
// uploads asynchronously.
//
// blacklist is a list of globs of files to ignore.
func PushDirectory(a Archiver, root string, blacklist []string) Future {
	if strings.HasSuffix(root, string(filepath.Separator)) {
		root = root[:len(root)-1]
	}
	rootLen := len(root) + 1

	i := isolated.Isolated{
		Algo:    "sha-1",
		Files:   map[string]isolated.File{},
		Version: isolated.IsolatedFormatVersion,
	}
	futures := map[string]Future{}
	// Directory enumeration is done synchronously.
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("walk(%s): %s", p, err)
		}
		if info.IsDir() {
			return nil
		}

		relPath := p[rootLen:]
		if filepath.Separator == '\\' {
			// Windows.
			relPath = strings.Replace(relPath, "\\", "/", -1)
		}

		mode := info.Mode()
		if mode&os.ModeSymlink == os.ModeSymlink {
			l, err := os.Readlink(p)
			if err != nil {
				return fmt.Errorf("readlink(%s): %s", p, err)
			}
			i.Files[relPath] = isolated.File{Link: newString(l)}
		} else {
			i.Files[relPath] = isolated.File{
				Mode: newInt(int(mode.Perm())),
				Size: newInt64(info.Size()),
			}
			futures[relPath] = a.PushFile(p)
		}
		return nil
	})

	s := &simpleFuture{err: err}
	if err != nil {
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
