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
	"io"

	"github.com/luci/luci-go/logdog/client/butlerlib/streamproto"
)

// StreamServer is an interface to a backgound service that allows external
// processes to establish Butler streams.
type StreamServer interface {
	// Performs initial connection and setup, entering a listening state.
	Listen() error

	// Address returns a string that can be used by the "streamclient" package to
	// return a client for this StreamServer.
	//
	// Full package is:
	// github.com/luci/luci-go/logdog/butlerlib/streamclient
	//
	// Address may only be called while the StreamServer is actively listening.
	Address() string

	// Blocks, returning a new Stream when one is available. If the stream server
	// has closed, this will return nil.
	Next() (io.ReadCloser, *streamproto.Properties)

	// Closes the stream server, cleaning up resources.
	Close()
}
