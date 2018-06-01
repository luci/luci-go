// Copyright 2015 The LUCI Authors.
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

package archiver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"go.chromium.org/luci/client/internal/common"
	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/isolatedclient"
	"go.chromium.org/luci/common/runtime/tracer"
)

type walkItem struct {
	fullPath string
	relPath  string
	info     os.FileInfo
}

// walk() enumerates a directory tree synchronously and sends the items to
// channel c.
func walk(root string, fsView common.FilesystemView, c chan<- *walkItem) {
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

	total := 0
	end := tracer.Span(root, "walk:"+filepath.Base(root), nil)
	defer func() { end(tracer.Args{"root": root, "total": total}) }()

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		total++

		if err != nil {
			return fmt.Errorf("walk(%q): %v", path, err)
		}

		relPath, err := fsView.RelativePath(path)
		if err != nil {
			return fmt.Errorf("walk(%q): %v", path, err)
		}

		if relPath == "" { // empty string indicates skip.
			return common.WalkFuncSkipFile(info)
		}

		if !info.IsDir() {
			c <- &walkItem{fullPath: path, relPath: relPath, info: info}
		}
		return nil
	})

	if err != nil {
		// No point continuing if an error occurred during walk.
		log.Fatalf("Unable to walk %q: %v", root, err)
	}

}

// PushDirectory walks a directory at root and creates a .isolated file.
//
// It walks the directories synchronously, then returns a *PendingItem to
// signal when the background work is completed. The PendingItem is signaled
// once all files are hashed. In particular, the *PendingItem is signaled
// before server side cache lookups and upload is completed. Use
// archiver.Close() to wait for completion.
//
// relDir is a relative directory to offset relative paths against in the
// generated .isolated file.
//
// blacklist is a list of globs of files to ignore.
func PushDirectory(a *Archiver, root string, relDir string, blacklist []string) *PendingItem {
	total := 0
	end := tracer.Span(a, "PushDirectory", tracer.Args{"path": relDir, "root": root})
	defer func() { end(tracer.Args{"total": total}) }()
	c := make(chan *walkItem)

	displayName := filepath.Base(root) + ".isolated"
	s := &PendingItem{DisplayName: displayName}
	fsView, err := common.NewFilesystemView(root, blacklist)
	if err != nil {
		s.SetErr(err)
		return s
	}

	go func() {
		walk(root, fsView, c)
		close(c)
	}()

	i := isolated.Isolated{
		Algo:    "sha-1",
		Files:   map[string]isolated.File{},
		Version: isolated.IsolatedFormatVersion,
	}
	items := []*PendingItem{}
	for item := range c {
		if s.Error() != nil {
			// Empty the queue.
			continue
		}
		total++
		if relDir != "" {
			item.relPath = filepath.Join(relDir, item.relPath)
		}
		mode := item.info.Mode()
		if mode&os.ModeSymlink == os.ModeSymlink {
			l, err := os.Readlink(item.fullPath)
			if err != nil {
				s.SetErr(fmt.Errorf("readlink(%s): %s", item.fullPath, err))
				continue
			}
			i.Files[item.relPath] = isolated.SymLink(l)
		} else {
			i.Files[item.relPath] = isolated.BasicFile("", int(mode.Perm()), item.info.Size())
			items = append(items, a.PushFile(item.relPath, item.fullPath, -item.info.Size()))
		}
	}
	if s.Error() != nil {
		return s
	}
	log.Printf("PushDirectory(%s) = %d files", root, len(i.Files))

	// Hashing, cache lookups and upload is done asynchronously.
	s.wgHashed.Add(1)
	go func() {
		defer s.wgHashed.Done()
		var err error
		for _, item := range items {
			item.WaitForHashed()
			if err = item.Error(); err != nil {
				break
			}
			name := item.DisplayName
			d := i.Files[name]
			d.Digest = item.Digest()
			i.Files[name] = d
		}
		if err == nil {
			raw := &bytes.Buffer{}
			if err = json.NewEncoder(raw).Encode(i); err == nil {
				if f := a.Push(displayName, isolatedclient.NewBytesSource(raw.Bytes()), 0); f != nil {
					f.WaitForHashed()
					if err = f.Error(); err == nil {
						s.lock.Lock()
						s.digestItem.Digest = string(f.Digest())
						s.lock.Unlock()
					}
				}
			}
		}
		if err != nil {
			s.SetErr(err)
		}
	}()
	return s
}
