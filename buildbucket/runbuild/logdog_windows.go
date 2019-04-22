// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package runbuild

import (
	"context"
	"fmt"
	"os"

	"go.chromium.org/luci/logdog/client/butler/streamserver"
)

// newLogDogStreamServerForPlatform creates a StreamServer instance usable on
// Windows.
func newLogDogStreamServerForPlatform(ctx context.Context, workDir string) (streamserver.StreamServer, error) {
	// Windows, use named pipe.
	return streamserver.NewNamedPipeServer(ctx, fmt.Sprintf("LUCILogDogRunBuild_%d", os.Getpid()))
}
