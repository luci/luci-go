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

	"go.chromium.org/luci/client/archiver"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/isolatedclient"
	"go.chromium.org/luci/common/logging"
)

// ArchiveOptions for achiving trees.
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

	// Blacklist is a list of filename regexes describing which files to
	// ignore when crawling the directories in Dirs.
	//
	// Note that this Blacklist will not filter files in Files.
	Blacklist []string

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
func Archive(c context.Context, arch *archiver.Archiver, opts *ArchiveOptions) *archiver.PendingItem {
	item, err := archive(c, arch, opts)
	if err != nil {
		arch.Cancel(err)
		i := &archiver.PendingItem{DisplayName: opts.Isolated}
		i.SetErr(err)
		return i
	}
	return item
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

	// Archive all directories.
	dItems := make(itemToPathMap, len(opts.Dirs))
	for dir, wd := range opts.Dirs {
		path := filepath.Join(wd, dir)
		dItems[archiver.PushDirectory(arch, path, dir, opts.Blacklist)] = path
	}

	// Construct isolated file.
	composite := isolated.New()
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
	err = waitOnItems(dItems, func(_, _ string, digest isolated.HexDigest) error {
		composite.Includes = append(composite.Includes, digest)
		return nil
	})
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
