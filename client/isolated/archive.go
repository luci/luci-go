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
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"

	"golang.org/x/net/context"

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
func Archive(c context.Context, arch *archiver.Archiver, opts *ArchiveOptions) *archiver.Item {
	item, err := archive(c, arch, opts)
	if err != nil {
		arch.Cancel(err)
		i := &archiver.Item{DisplayName: opts.Isolated}
		i.SetErr(err)
		return i
	}
	return item
}

func archive(c context.Context, arch *archiver.Archiver, opts *ArchiveOptions) (*archiver.Item, error) {
	// Archive all files.
	fItems := make(map[*archiver.Item]string, len(opts.Files))
	for file, wd := range opts.Files {
		fItems[arch.PushFile(file, filepath.Join(wd, file), 0)] = wd
	}

	// Archive all directories.
	dItems := make(map[*archiver.Item]string, len(opts.Dirs))
	for dir, wd := range opts.Dirs {
		dItems[archiver.PushDirectory(arch, filepath.Join(wd, dir), dir, opts.Blacklist)] = wd
	}

	// Construct isolated file.
	composite := isolated.New()
	err := waitOnItems(fItems, func(wd, file string, digest isolated.HexDigest) error {
		f, err := isolatedFile(filepath.Join(wd, file), digest)
		if err != nil {
			return err
		}
		composite.Files[file] = f
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

func isolatedFile(file string, digest isolated.HexDigest) (isolated.File, error) {
	info, err := os.Lstat(file)
	if err != nil {
		return isolated.File{}, err
	}
	mode := info.Mode()
	if mode&os.ModeSymlink == os.ModeSymlink {
		l, err := os.Readlink(file)
		if err != nil {
			return isolated.File{}, err
		}
		return isolated.SymLink(l), nil
	}
	return isolated.BasicFile(digest, int(mode.Perm()), info.Size()), nil
}

func waitOnItems(items map[*archiver.Item]string, cb func(string, string, isolated.HexDigest) error) error {
	for item, wd := range items {
		item.WaitForHashed()
		if err := item.Error(); err != nil {
			return errors.Annotate(err, "%s failed\n", item.DisplayName).Err()
		}
		digest := item.Digest()
		if err := cb(wd, item.DisplayName, digest); err != nil {
			return err
		}
	}
	return nil
}
