// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package cipd

import (
	"os"

	"github.com/luci/luci-go/cipd/client/cipd/local"
	"golang.org/x/net/context"
)

func (client *clientImpl) installClient(ctx context.Context, fs local.FileSystem, fetchURL, destination string) error {
	return fs.EnsureFile(ctx, destination, func(of *os.File) error {
		// TODO(iannucci): worry about owner/group/permissions?
		return client.storage.download(ctx, fetchURL, of)
	})
}
