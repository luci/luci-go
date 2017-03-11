// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

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
	return fs.EnsureFile(ctx, destination, func(of *os.File) error {
		// TODO(iannucci): worry about owner/group/permissions?
		if err := client.storage.download(ctx, fetchURL, of, h); err != nil {
			return err
		}
		if hex.EncodeToString(h.Sum(nil)) != digest {
			return fmt.Errorf("file hash mismatch")
		}
		return nil
	})
}
