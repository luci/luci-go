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
	"encoding/json"
	"fmt"
	"io"

	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"
	"go.chromium.org/luci/logdog/common/types"
)

// Client is a client to a LogDog Butler StreamServer. A Client will connect
// to a StreamServer, negotiate a stream configuration, and return an active
// stream object that can be written to.
type Client interface {
	// NewStream creates a new stream with the supplied stream properties.
	NewStream(f streamproto.Flags) (Stream, error)
}

// streamFactory is a factory method to generate an io.WriteCloser stream for
// the current clientImpl.
type streamFactory func() (io.WriteCloser, error)

// clientImpl is an implementation of the Client interface using a net.Conn
// factory to generate an individual stream.
type clientImpl struct {
	// network is the connection path to the stream server.
	factory streamFactory

	// namespace for all streams created in this client
	ns types.StreamName
}

// New instantiates a new Client instance. This type of instance will be parsed
// from the supplied path string, which takes the form:
//   <protocol>:<protocol-specific-spec>
//
// Supported protocols and their respective specs are:
//   - unix:/path/to/socket describes a stream server listening on UNIX domain
//     socket at "/path/to/socket".
//
// Windows-only:
//   - net.pipe:name describes a stream server listening on Windows named pipe
//     "\\.\pipe\name".
func New(path string, ns types.StreamName) (Client, error) {
	return GetDefaultRegistry().NewClient(path, ns)
}

func (c *clientImpl) NewStream(f streamproto.Flags) (Stream, error) {
	f.Name = streamproto.StreamNameFlag(c.ns.Concat(types.StreamName(f.Name)))

	p := f.Properties()
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("streamclient: invalid stream properties: %s", err)
	}

	client, err := c.factory()
	if err != nil {
		return nil, err
	}
	ownsClient := true
	defer func() {
		// If we haven't written out the connection, close.
		if ownsClient {
			client.Close()
		}
	}()

	data, err := json.Marshal(f)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal properties JSON: %s", err)
	}

	// Perform the handshake: magic + size(data) + data.
	s := &BaseStream{
		WriteCloser: client,
		P:           p,
	}
	if _, err := s.writeRaw(streamproto.ProtocolFrameHeaderMagic); err != nil {
		return nil, fmt.Errorf("failed to write magic number: %s", err)
	}
	if err := s.writeRecord(data); err != nil {
		return nil, fmt.Errorf("failed to write properties: %s", err)
	}

	ownsClient = false // Passing ownership to caller.
	return s, nil
}
