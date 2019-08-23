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
	"io"
	"strings"
	"sync"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"
	"go.chromium.org/luci/logdog/common/types"
)

// fakeStream represents a dialed stream, and has methods to retrieve the data
// written to it.
type fakeStream struct {
	flags streamproto.Flags

	mu         sync.RWMutex
	closed     bool
	streamData strings.Builder
	datagrams  []string
}

var _ interface {
	DatagramStream
	io.Writer
	FakeStreamData
} = (*fakeStream)(nil)

func (ds *fakeStream) Close() error {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	if ds.closed {
		return io.ErrClosedPipe
	}
	ds.closed = true
	return nil
}

func (ds *fakeStream) Write(data []byte) (int, error) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	if ds.closed {
		return 0, io.ErrClosedPipe
	}
	return ds.streamData.Write(data)
}

func (ds *fakeStream) WriteDatagram(dg []byte) error {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	if ds.closed {
		return io.ErrClosedPipe
	}
	ds.datagrams = append(ds.datagrams, string(dg))
	return nil
}

func (ds *fakeStream) GetFlags() streamproto.Flags {
	return ds.flags
}

func (ds *fakeStream) GetStreamData() string {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.streamData.String()
}

func (ds *fakeStream) GetDatagrams() []string {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return append(make([]string, 0, len(ds.datagrams)), ds.datagrams...)
}

func (ds *fakeStream) IsClosed() bool {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.closed
}

type fakeDialer struct {
	mu      sync.RWMutex
	err     error
	streams map[types.StreamName]*fakeStream
}

var _ dialer = (*fakeDialer)(nil)

// Makes a shallow interface-based map of all of the fakeStreams.
//
// Because the FakeStreamData interface is read-only, this should be safe.
func (d *fakeDialer) dumpFakeData() map[types.StreamName]FakeStreamData {
	d.mu.RLock()
	defer d.mu.RUnlock()
	ret := make(map[types.StreamName]FakeStreamData, len(d.streams))
	for k, v := range d.streams {
		ret[k] = v
	}
	return ret
}

func (d *fakeDialer) setFakeError(err error) {
	d.mu.Lock()
	d.err = err
	d.mu.Unlock()
}

func (d *fakeDialer) getStream(f streamproto.Flags) (*fakeStream, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.err != nil {
		return nil, d.err
	}
	streamName := types.StreamName(f.Name)
	if _, hasStream := d.streams[streamName]; hasStream {
		return nil, errors.Reason("stream %q already dialed", streamName).Err()
	}

	ret := &fakeStream{flags: f}
	d.streams[streamName] = ret
	return ret, nil
}

func (d *fakeDialer) DialStream(forProcess bool, f streamproto.Flags) (io.WriteCloser, error) {
	return d.getStream(f)
}

func (d *fakeDialer) DialDgramStream(f streamproto.Flags) (DatagramStream, error) {
	return d.getStream(f)
}

// NewFake makes a FakeClient which absorbs all data written to the client, and
// allows retrieval via extra methods.
func NewFake(namespace types.StreamName) FakeClient {
	return FakeClient{
		Client: &Client{
			&fakeDialer{streams: map[types.StreamName]*fakeStream{}},
			namespace,
		},
	}
}

// FakeClient implements a client which doesn't connect to a real server, but
// allows the retrieval of the dialed data later via GetFakeData.
type FakeClient struct {
	*Client
}

// FakeStreamData is data about a single stream for a client created with the
// "fake" protocol. You can retrieve this with Client.GetFakeData().
type FakeStreamData interface {
	// GetFlags returns the stream's streamproto.Flags from when the stream was
	// opened.
	GetFlags() streamproto.Flags

	// GetStreamData returns the written text/binary data as a string.
	//
	// If this is a datagram stream, returns "".
	GetStreamData() string

	// GetDatagrams returns the written datagrams as a list of strings.
	//
	// If this is a text/binary stream, returns nil.
	GetDatagrams() []string

	// Returns true iff the user of the client has closed this stream.
	IsClosed() bool
}

// GetFakeData returns all absorbed data collected by this client so far.
//
// Note that the FakeStreamData objects are `live`, and so calling their methods
// will always get you the current value of those streams. You'll need to call
// this method again if you expect a new stream to show up, however.
func (c FakeClient) GetFakeData() map[types.StreamName]FakeStreamData {
	return c.dial.(*fakeDialer).dumpFakeData()
}

// SetFakeError causes New* methods on this Client to return this error.
func (c FakeClient) SetFakeError(err error) {
	c.dial.(*fakeDialer).setFakeError(err)
}
