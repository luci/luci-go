// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// +build !windows

package cipd

import (
	"os"

	"github.com/luci/luci-go/cipd/client/cipd/local"
	"golang.org/x/net/context"
)

func (client *clientImpl) installClient(ctx context.Context, fs local.FileSystem, fetchURL, destination string) error {
	curStat, err := os.Stat(destination)
	if err != nil {
		return err
	}

	return fs.EnsureFile(ctx, destination, func(of *os.File) error {
		if err := of.Chmod(curStat.Mode()); err != nil {
			return err
		}
		// TODO(iannucci): worry about owner/group?
		return client.storage.download(ctx, fetchURL, of)
	})
}
