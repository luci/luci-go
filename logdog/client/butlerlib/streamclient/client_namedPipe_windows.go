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
