// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package streamclient

import (
	"errors"
	"io"

	"github.com/Microsoft/go-winio"
)

func registerPlatformProtocols(r *Registry) {
	r.Register("net.pipe", newNamedPipeClient)
}

// newNamedPipeClient creates a new Client instance bound to a named pipe stream
// server.
func newNamedPipeClient(path string) (Client, error) {
	if path == "" {
		return nil, errors.New("streamclient: cannot have empty named pipe path")
	}

	return &clientImpl{
		factory: func() (io.WriteCloser, error) {
			return winio.DialPipe(LocalNamedPipePath(path), nil)
		},
	}, nil
}

// LocalNamedPipePath returns the path to a local Windows named pipe named base.
func LocalNamedPipePath(base string) string {
	return `\\.\pipe\` + base
}
