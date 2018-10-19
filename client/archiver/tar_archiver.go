// Copyright 2016 The LUCI Authors.
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
	"archive/tar"
	"io"
	"os"
	"sort"

	"go.chromium.org/luci/common/iotools"
	"go.chromium.org/luci/common/isolated"
)

// osOpen wraps os.Open to allow faking out during tests.
var osOpen = func(name string) (io.ReadCloser, error) {
	return os.Open(name)
}

// itemBundle is a slice of *Item that will be archived together.
type itemBundle struct {
	items []*Item
	// itemSize is the total size (in bytes) of the constituent files. It will be
	// smaller than the resultant tar.
	itemSize int64
}

// shardItems shards the provided items into itemBundles, using the provided
// threshold as the maximum size the resultant tars should be.
//
// shardItems does not access the filesystem.
func shardItems(items []*Item, threshold int64) []*itemBundle {
	// For deterministic isolated hashes, sort the items by path.
	sort.Sort(itemByPath(items))

	var bundles []*itemBundle
	for len(items) > 0 {
		var bundle *itemBundle
		bundle, items = oneBundle(items, threshold)
		bundles = append(bundles, bundle)
	}
	return bundles
}

func oneBundle(items []*Item, threshold int64) (*itemBundle, []*Item) {
	bundle := &itemBundle{}
	bundleTarSize := int64(1024) // two trailing blank 512-byte records.

	for i, item := range items {
		// The in-tar size of the file (512 header + rounded up to nearest 512).
		tarSize := (item.Size + 1023) & ^511

		if i > 0 && bundleTarSize+tarSize > threshold {
			return bundle, items[i:]
		}

		bundle.items = items[:i+1]
		bundle.itemSize += item.Size
		bundleTarSize += tarSize
	}
	return bundle, nil
}

// Digest returns the hash and total size of the tar constructed from the
// bundle's items.
func (b *itemBundle) Digest() (isolated.HexDigest, int64, error) {
	h := isolated.GetHash()
	cw := &iotools.CountingWriter{Writer: h}
	if err := b.writeTar(cw); err != nil {
		return "", 0, err
	}
	return isolated.Sum(h), cw.Count, nil
}

// Contents returns an io.ReadCloser containing the tar's contents.
func (b *itemBundle) Contents() (io.ReadCloser, error) {
	pr, pw := io.Pipe()
	go func() {
		pw.CloseWithError(b.writeTar(pw))
	}()
	return pr, nil
}

func (b *itemBundle) writeTar(w io.Writer) error {
	tw := tar.NewWriter(w)

	for _, item := range b.items {
		if err := tw.WriteHeader(&tar.Header{
			Name:     item.RelPath,
			Mode:     int64(item.Mode),
			Typeflag: tar.TypeReg,
			Size:     item.Size,
		}); err != nil {
			return err
		}

		file, err := osOpen(item.Path)
		if err != nil {
			return err
		}
		_, err = io.Copy(tw, file)
		file.Close()
		if err != nil {
			return err
		}
	}
	return tw.Close()
}

// itemByPath implements sort.Interface through path-based comparison.
type itemByPath []*Item

func (s itemByPath) Len() int {
	return len(s)
}
func (s itemByPath) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s itemByPath) Less(i, j int) bool {
	return s[i].RelPath < s[j].RelPath
}
