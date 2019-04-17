// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// +build darwin dragonfly freebsd linux netbsd openbsd

package runbuild

import (
	"context"
	"path/filepath"

	"go.chromium.org/luci/logdog/client/butler/streamserver"
)

// newLogDogStreamServerForPlatform creates a StreamServer instance usable on
// POSIX.
func newLogDogStreamServerForPlatform(ctx context.Context, workDir string) (streamserver.StreamServer, error) {
	// POSIX, use UNIX domain socket.
	return streamserver.NewUNIXDomainSocketServer(ctx, filepath.Join(workDir, "ld.sock"))
}
