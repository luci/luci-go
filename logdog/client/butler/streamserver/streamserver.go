// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

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
