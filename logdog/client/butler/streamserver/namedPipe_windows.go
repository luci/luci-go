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
	"net"

	"go.chromium.org/luci/common/errors"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"

	"github.com/Microsoft/go-winio"
)

// maxWindowsNamedPipeLength is the maximum length of a Windows named pipe.
const maxWindowsNamedPipeLength = 256

// NewNamedPipeServer instantiates a new Windows named pipe server instance.
func NewNamedPipeServer(ctx context.Context, name string) (StreamServer, error) {
	switch l := len(name); {
	case l == 0:
		return nil, errors.New("cannot have empty name")
	case l > maxWindowsNamedPipeLength:
		return nil, errors.Reason("name exceeds maximum length %d", maxWindowsNamedPipeLength).
			InternalReason("name(%s)", name).Err()
	}

	ctx = log.SetField(ctx, "name", name)
	return &listenerStreamServer{
		Context: ctx,
		gen: func() (net.Listener, string, error) {
			address := "net.pipe:" + name
			pipePath := streamclient.LocalNamedPipePath(name)
			log.Fields{
				"addr":     address,
				"pipePath": pipePath,
			}.Debugf(ctx, "Creating Windows server socket Listener.")

			l, err := winio.ListenPipe(pipePath, nil)
			if err != nil {
				return nil, "", errors.Annotate(err, "failed to listen on named pipe").Err()
			}
			return l, address, nil
		},
	}, nil
}
