// Copyright 2017 The LUCI Authors.
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

package isolated

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"

	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/client/archiver"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/isolatedclient"
	"go.chromium.org/luci/common/logging"
)

// ArchiveOptions for archiving trees.
type ArchiveOptions struct {
	// Files is a map of working directories to files relative to that working
	// directory.
	//
	// The working directories may be relative to CWD.
	Files ScatterGather

	// Dirs is a map of working directories to directories relative to that
	// working directory.
	//
	// The working directories may be relative to CWD.
	Dirs ScatterGather

	// Isolated is the display name of the isolated to upload.
	Isolated string

	// LeakIsolated is handle to a place where Archive will write
	// the encoded bytes of the isolated file.
	LeakIsolated io.Writer
}

// Archive uses the given archiver and options to constructed an isolated file, and
// uploads it and its dependencies.
//
// Archive returns the digest of the composite isolated file.
func Archive(ctx context.Context, arch *archiver.Archiver, opts *ArchiveOptions) *archiver.PendingItem {
	item, err := archive(ctx, arch, opts)
	if err != nil {
		i := &archiver.PendingItem{DisplayName: opts.Isolated}
		i.SetErr(err)
		return i
	}
	return item
}

// ArchiveFiles uploads given files using given archiver.
//
// This is thin wrapper of Archive.
// Note that this function may have large number of concurrent RPCs to isolate server.
func ArchiveFiles(ctx context.Context, arch *archiver.Archiver, baseDir string, files []string) ([]*archiver.PendingItem, error) {
	items := make([]*archiver.PendingItem, len(files))

	var g errgroup.Group
	for i, file := range files {
		i, file := i, file
		g.Go(func() error {
			fi, err := os.Stat(filepath.Join(baseDir, file))
			if err != nil {
				return errors.Annotate(err, "failed to get stat for %s", file).Err()
			}

			fg := ScatterGather{}
			dg := ScatterGather{}

			if fi.IsDir() {
				if err := dg.Add(baseDir, file); err != nil {
					return errors.Annotate(err, "failed to add directory %s", file).Err()
				}
			} else if fi.Mode().IsRegular() {
				if err := fg.Add(baseDir, file); err != nil {
					return errors.Annotate(err, "failed to add file %s", file).Err()
				}
			} else {
				return errors.Annotate(err, "unsupported file type for %s: %s", file, fi).Err()
			}

			opts := ArchiveOptions{
				Files: fg,
				Dirs:  dg,
			}

			items[i] = Archive(ctx, arch, &opts)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return items, nil
}

// Convenience type to track pending items and their corresponding filepaths.
type itemToPathMap map[*archiver.PendingItem]string

func archive(c context.Context, arch *archiver.Archiver, opts *ArchiveOptions) (*archiver.PendingItem, error) {
	// Archive all files.
	fItems := make(itemToPathMap, len(opts.Files))
	for file, wd := range opts.Files {
		path := filepath.Join(wd, file)
		info, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		mode := info.Mode()
		if mode&os.ModeSymlink == os.ModeSymlink {
			path, err = filepath.EvalSymlinks(path)
			if err != nil {
				return nil, err
			}
		}
		fItems[arch.PushFile(file, path, 0)] = path
	}

	// Construct isolated file.
	composite := isolated.New(arch.Hash())

	// Archive all directories.
	for dir, wd := range opts.Dirs {
		path := filepath.Join(wd, dir)
		dirFItems, dirSymItems, err := archiver.PushDirectory(arch, path, dir)
		if err != nil {
			return nil, err
		}

		for pending, item := range dirFItems {
			fItems[pending] = item.FullPath
		}
		for relPath, dst := range dirSymItems {
			composite.Files[relPath] = isolated.SymLink(dst)
		}
	}

	err := waitOnItems(fItems, func(path, file string, digest isolated.HexDigest) error {
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		mode := info.Mode()
		composite.Files[file] = isolated.BasicFile(digest, int(mode.Perm()), info.Size())
		logging.Infof(c, "%s %s", digest, file)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	sort.Sort(composite.Includes)

	// Serialize isolated file.
	var rawComposite bytes.Buffer
	if err := json.NewEncoder(&rawComposite).Encode(composite); err != nil {
		return nil, err
	}
	compositeBytes := rawComposite.Bytes()

	// Archive isolated file.
	compositeItem := arch.Push(opts.Isolated, isolatedclient.NewBytesSource(compositeBytes), 0)

	// Write isolated file somewhere if necessary.
	if opts.LeakIsolated != nil {
		if _, err := opts.LeakIsolated.Write(compositeBytes); err != nil {
			return nil, errors.Annotate(err, "failed to leak isolated").Err()
		}
	}
	return compositeItem, nil
}

func waitOnItems(items itemToPathMap, cb func(string, string, isolated.HexDigest) error) error {
	for item, path := range items {
		item.WaitForHashed()
		if err := item.Error(); err != nil {
			return errors.Annotate(err, "%s failed\n", item.DisplayName).Err()
		}
		digest := item.Digest()
		if err := cb(path, item.DisplayName, digest); err != nil {
			return err
		}
	}
	return nil
}
