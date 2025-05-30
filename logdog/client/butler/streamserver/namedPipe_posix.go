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

//go:build unix
// +build unix

package streamserver

import (
	"context"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	log "go.chromium.org/luci/common/logging"
)

// maxPOSIXNamedSocketLength is the maximum length of a UNIX domain socket.
//
// This is defined by the UNIX_PATH_MAX constant, and is usually this value.
const maxPOSIXNamedSocketLength = 104

// newStreamServer instantiates a new POSIX domain socket server
// instance.
//
// No resources are actually created until methods are called on the returned
// server.
func newStreamServer(ctx context.Context, path string) (*StreamServer, error) {
	if path == "" {
		tFile, err := ioutil.TempFile("", "ld")
		if err != nil {
			return nil, err
		}
		path = tFile.Name()
		if err := tFile.Close(); err != nil {
			return nil, errors.Fmt("closing tempfile %q: %w", path, err)
		}
	} else {
		abs, err := filepath.Abs(path)
		if err != nil {
			return nil, errors.Fmt("could not get absolute path of %q: %w", path, err)
		}
		path = abs
	}

	if len(path) > maxPOSIXNamedSocketLength {
		return nil, errors.Fmt("path exceeds maximum length %d", maxPOSIXNamedSocketLength)
	}

	ctx = log.SetField(ctx, "namedPipePath", path)
	return &StreamServer{
		log:     logging.Get(ctx),
		address: "unix:" + path,
		gen: func() (listener, error) {
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
				return nil, err
			}

			ul := selfCleaningUNIXListener{
				Context:  ctx,
				Listener: l,
				path:     path,
			}
			return mkListener(&ul), nil
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
