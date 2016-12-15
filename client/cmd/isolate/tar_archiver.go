// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"archive/tar"
	"crypto/sha1"
	"io"
	"os"

	"github.com/luci/luci-go/common/isolated"
)

// osOpen wraps os.Open to allow faking out during tests.
var osOpen = func(name string) (io.ReadCloser, error) {
	return os.Open(name)
}

// ItemBundle is a slice of *Items that will be archived together.
type ItemBundle struct {
	Items []*Item
	// ItemSize is the total size (in bytes) of the constituent files. It will be
	// smaller than the resultant tar.
	ItemSize int64
	// TarSize is the size (in bytes) of the resultant tar.
	TarSize int64
}

// ShardItems shards the provided items into ItemBundles, using the provided
// threshold as the maximum size the resultant tars should be.
//
// ShardItems does not access the filesystem to determine
func ShardItems(items []*Item, threshold int64) []*ItemBundle {
	var (
		bundles []*ItemBundle
		bundle  *ItemBundle
	)

	for len(items) > 0 {
		bundle, items = oneBundle(items, threshold)
		bundles = append(bundles, bundle)
	}
	return bundles
}

func oneBundle(items []*Item, threshold int64) (*ItemBundle, []*Item) {
	bundle := &ItemBundle{
		TarSize: 1024, // two trailing blank 512-byte records.
	}

	for i, item := range items {
		// The in-tar size of the file (512 header + rounded up to nearest 512).
		tarSize := (item.Size + 1023) & ^511

		if i > 0 && bundle.TarSize+tarSize > threshold {
			return bundle, items[i:]
		}

		bundle.Items = items[:i+1]
		bundle.ItemSize += item.Size
		bundle.TarSize += tarSize
	}
	return bundle, nil
}

// Digest returns the hash of the tar constructed from the bundle's items.
func (b *ItemBundle) Digest() (isolated.HexDigest, error) {
	h := sha1.New()
	if err := b.writeTar(h); err != nil {
		return "", err
	}
	return isolated.Sum(h), nil
}

// Contents returns an io.ReadCloser containing the tar's contents.
func (b *ItemBundle) Contents() (io.ReadCloser, error) {
	pr, pw := io.Pipe()
	go func() {
		pw.CloseWithError(b.writeTar(pw))
	}()
	return pr, nil
}

func (b *ItemBundle) writeTar(w io.Writer) error {
	tw := tar.NewWriter(w)

	for _, item := range b.Items {
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
