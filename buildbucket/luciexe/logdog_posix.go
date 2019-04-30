// Copyright 2019 The LUCI Authors.
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

// +build darwin dragonfly freebsd linux netbsd openbsd

package luciexe

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
