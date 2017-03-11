// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// +build !windows

package cipd

import (
	"encoding/hex"
	"fmt"
	"hash"
	"os"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/cipd/client/cipd/local"
)

func (client *clientImpl) installClient(ctx context.Context, fs local.FileSystem, h hash.Hash, fetchURL, destination, digest string) error {
	curStat, err := os.Stat(destination)
	if err != nil {
		return err
	}

	return fs.EnsureFile(ctx, destination, func(of *os.File) error {
		if err := of.Chmod(curStat.Mode()); err != nil {
			return err
		}
		// TODO(iannucci): worry about owner/group?
		if err := client.storage.download(ctx, fetchURL, of, h); err != nil {
			return err
		}
		if hex.EncodeToString(h.Sum(nil)) != digest {
			return fmt.Errorf("file hash mismatch")
		}
		return nil
	})
}
