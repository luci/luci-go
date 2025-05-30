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

package streamserver

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"

	"github.com/Microsoft/go-winio"
)

// maxWindowsNamedPipeLength is the maximum length of a Windows named pipe.
const maxWindowsNamedPipeLength = 256

var winpipeCounter uint64

const defaultWinPipePrefix = "logdog_butler"

// newStreamServer instantiates a new Windows named pipe server instance.
func newStreamServer(ctx context.Context, prefix string) (*StreamServer, error) {
	if prefix == "" {
		prefix = defaultWinPipePrefix
	}

	path := fmt.Sprintf("%s.%d.%d", prefix, os.Getpid(), atomic.AddUint64(&winpipeCounter, 1))
	realPath := streamproto.LocalNamedPipePath(path)

	if len(realPath) > maxWindowsNamedPipeLength {
		return nil, errors.Fmt("path exceeds maximum length %d", maxWindowsNamedPipeLength)
	}

	ctx = log.SetField(ctx, "path", path)
	return &StreamServer{
		log:     logging.Get(ctx),
		address: "net.pipe:" + path,
		gen: func() (listener, error) {
			log.Infof(ctx, "Creating Windows server socket Listener.")

			l, err := winio.ListenPipe(realPath, &winio.PipeConfig{
				InputBufferSize:  1024 * 1024,
				OutputBufferSize: 1024 * 1024,
			})
			if err != nil {
				return nil, errors.Fmt("failed to listen on named pipe: %w", err)
			}
			return mkListener(l), nil
		},
	}, nil
}
