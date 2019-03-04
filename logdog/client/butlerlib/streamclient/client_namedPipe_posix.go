// Copyright 2015 The LUCI Authors.
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

package streamclient

import (
	"fmt"
	"io"
	"net"
	"os"

	"go.chromium.org/luci/logdog/common/types"
)

func registerPlatformProtocols(r *Registry) {
	r.Register("unix", newUnixSocketClient)
}

// newUnixSocketClient creates a new Client instance bound to a named pipe stream
// server.
//
// newNPipeClient currently only works on POSIX (non-Windows) systems.
func newUnixSocketClient(path string, ns types.StreamName) (Client, error) {
	// Ensure that the supplied path exists and is a named pipe.
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file [%s]: %s", path, err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		return nil, fmt.Errorf("not a named pipe: [%s]", path)
	}

	return &clientImpl{
		factory: func() (io.WriteCloser, error) {
			return net.Dial("unix", path)
		},
		ns: ns,
	}, nil
}
