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

package streamclient

import (
	"context"
	"io"
	"strings"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/clockflag"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"
	"go.chromium.org/luci/logdog/common/types"
)

// This is populated via init() functions in this package.
var protocolRegistry = map[string]dialFactory{}

// dialFactory takes an implementation-specific address (e.g. `localhost:1234`
// or `/path/to/fifo`), and returns a `dialer` function which can be invoked to
// open a new, raw, connection to the butler.
type dialFactory func(addr string) (dialer, error)

// dialer is called by Client for every new stream created, and is expected to:
//   - open a connection (as appropriate) to the dialer's destination
//   - "handshake" with the opened connection (if needed)
//   - return an appropriate writer type around the connection.
type dialer interface {
	// if forProcess is true, this must do its best to return an *os.File.
	//
	// If the implementation fails to do so, then it will cause "os/exec" to
	// allocate an extra goroutine and copy loop when this stream is attached to
	// a command's stdout/stderr.
	DialStream(forProcess bool, f streamproto.Flags) (io.WriteCloser, error)
	DialDgramStream(f streamproto.Flags) (DatagramStream, error)
}

// Client is a client to a local LogDog Butler.
//
// The methods here allow you to open a stream (text, binary or datagram) which
// you can then use to send data to LogDog.
type Client struct {
	dial dialer

	ns types.StreamName
}

// New instantiates a new Client instance. This type of instance will be parsed
// from the supplied path string, which takes the form:
//
//	<protocol>:<protocol-specific-spec>
//
// # Supported protocols
//
// Below is the list of all supported protocols:
//
//	unix:/path/to/socket (POSIX only)
//
// Connects to a UNIX domain socket at "/path/to/socket".
// This is the preferred protocol for Linux/Mac.
//
//	net.pipe:name (Windows only)
//
// Connects to a local Windows named pipe "\\.\pipe\name". This is the preferred
// protocol for Windows.
//
//	null (All platforms)
//
// Sinks all connections and writes into a null data sink. Useful for tests, or
// for running programs which use logdog but you don't care about their logdog
// outputs.
//
//	fake:$id
//
// Connects to an in-memory Fake created in this package by calling NewFake.
// The string `fake:$id` can be obtained by calling the Fake's StreamServerPath
// method.
func New(path string, namespace types.StreamName) (*Client, error) {
	parts := strings.SplitN(path, ":", 2)
	protocol, value := parts[0], ""
	if len(parts) == 2 {
		value = parts[1]
	}

	if f, ok := protocolRegistry[protocol]; ok {
		dial, err := f(value)
		if err != nil {
			return nil, errors.Fmt("opening path %q: %w", path, err)
		}
		return &Client{dial, namespace}, nil
	}
	return nil, errors.Fmt("no protocol registered for [%s]", parts[0])
}

func (c *Client) mkOptions(ctx context.Context, name types.StreamName, opts []Option) (ret options, err error) {
	ret.desc, ret.forProcess = RenderOptions(opts...)
	ret.desc.Name = streamproto.StreamNameFlag(c.ns.Concat(name))
	if time.Time(ret.desc.Timestamp).IsZero() {
		ret.desc.Timestamp = clockflag.Time(clock.Now(ctx))
	}
	if ret.desc.ContentType == "" {
		ret.desc.ContentType = string(ret.desc.Type.DefaultContentType())
	}

	if err = ret.desc.Descriptor().Validate(false); err != nil {
		err = errors.Fmt("invalid stream descriptor: %w", err)
	}
	return
}

// NewStream returns a new open stream to the butler.
//
// By default this is text-based (line-oriented); pass Binary for a binary stream.
//
// Text streams look for newlines to delimit log chunks.
// Binary streams use fixed size chunks to delimit log chunks.
func (c *Client) NewStream(ctx context.Context, name types.StreamName, opts ...Option) (io.WriteCloser, error) {
	fullOpts, err := c.mkOptions(ctx, name, opts)
	if err != nil {
		return nil, err
	}
	ret, err := c.dial.DialStream(fullOpts.forProcess, fullOpts.desc)
	return ret, errors.WrapIf(err, "attempting to connect stream %q", name)
}

// NewDatagramStream returns a new datagram stream to the butler.
//
// Datagram streams allow you to send messages without having to demark the
// separation between messages.
//
// NOTE: It is an error to pass ForProcess as an Option (see documentation on
// ForProcess for more detail).
func (c *Client) NewDatagramStream(ctx context.Context, name types.StreamName, opts ...Option) (DatagramStream, error) {
	newOpts := make([]Option, 0, len(opts)+1)
	newOpts = append(newOpts, opts...)
	newOpts = append(newOpts, func(o *options) {
		o.desc.Type = streamproto.StreamType(logpb.StreamType_DATAGRAM)
	})

	fullOpts, err := c.mkOptions(ctx, name, newOpts)
	if err != nil {
		return nil, err
	}
	if fullOpts.forProcess {
		return nil, errors.New("cannot specify ForProcess on a datagram stream")
	}
	ret, err := c.dial.DialDgramStream(fullOpts.desc)
	return ret, errors.WrapIf(err, "attempting to connect datagram stream %q", name)
}

// GetNamespace returns the LOGDOG_NAMESPACE value associated with this Client.
//
// Safe to call on a nil client; will return an empty StreamName.
func (c *Client) GetNamespace() types.StreamName {
	if c == nil {
		return types.StreamName("")
	}
	return c.ns
}
