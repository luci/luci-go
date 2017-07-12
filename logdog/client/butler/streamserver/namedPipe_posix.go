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

package streamserver

import (
	"net"
	"os"
	"path/filepath"

	"github.com/luci/luci-go/common/errors"
	log "github.com/luci/luci-go/common/logging"

	"golang.org/x/net/context"
)

// maxPOSIXNamedSocketLength is the maximum length of a UNIX domain socket.
//
// This is defined by the UNIX_PATH_MAX constant, and is usually this value.
const maxPOSIXNamedSocketLength = 104

// NewUNIXDomainSocketServer instantiates a new POSIX domain soecket server
// instance.
//
// No resources are actually created until methods are called on the returned
// server.
func NewUNIXDomainSocketServer(ctx context.Context, path string) (StreamServer, error) {
	switch l := len(path); {
	case l == 0:
		return nil, errors.New("cannot have empty path")
	case l > maxPOSIXNamedSocketLength:
		return nil, errors.Reason("path exceeds maximum length %d", maxPOSIXNamedSocketLength).
			InternalReason("path(%s)", path).Err()
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, errors.Annotate(err, "could not get absolute path of [%s]", path).Err()
	}
	path = abs

	ctx = log.SetField(ctx, "namedPipePath", path)
	return &listenerStreamServer{
		Context: ctx,
		gen: func() (net.Listener, string, error) {
			log.Infof(ctx, "Creating POSIX server socket Listener.")

			// Cleanup any previous named pipe. We don't bother checking for the file
			// first since the remove is atomic. We also ignore any error here, since
			// it's probably related to the file not being found.
			//
			// If there was an actual error removing the file, we'll catch it shortly
			// when we try to create it.
			os.Remove(path)

			// Create a UNIX listener
			l, err := net.Listen("unix", path)
			if err != nil {
				return nil, "", err
			}

			addr := "unix:" + path
			ul := selfCleaningUNIXListener{
				Context:  ctx,
				Listener: l,
				path:     path,
			}
			return &ul, addr, nil
		},
	}, nil
}

// selfCleaningUNIXListener is a wrapper around the "unix"-type Listener that
// cleans up the named pipe on creation and Close().
//
// The standard Go Listener will unlink the file when Closed. However, it
// doesn't do it in a deferred, so this will clean up if a panic is encountered
// during close.
type selfCleaningUNIXListener struct {
	context.Context
	net.Listener

	path string
}

func (l *selfCleaningUNIXListener) Close() error {
	defer func() {
		if err := os.Remove(l.path); err != nil {
			log.Fields{
				log.ErrorKey: err,
			}.Debugf(l, "Failed to remove named pipe file on Close().")
		}
	}()

	if err := l.Listener.Close(); err != nil {
		return err
	}
	return nil
}
