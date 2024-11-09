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
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

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

// These support NewFake and the "fake" protocol handler.
var (
	fakeRegistryMu sync.Mutex
	fakeRegistryID int64
	fakeRegistry   = map[int64]Fake{}
)

// NewFake makes an in-memory Fake which absorbs all data written via Clients
// connected to it, and allows retrieval of this data via extra methods.
//
// You must call Fake.Unregister to free the collected data.
//
// You can obtain a Client for this Fake by doing:
//
//	fake := streamclient.NewFake()
//	defer fake.Unregister()
//	client, _ := streamclient.New(fake.StreamServerPath(), "whatever")
//
// This is particularly useful for testing programs which use
// bootstrap.GetFromEnv() to obtain the Client:
//
//	env := environ.New(nil)
//	env.Set(bootstrap.EnvStreamServerPath, client.StreamServerPath())
//
//	bs := bootstrap.GetFromEnv(env)   # returns Bootstrap containing `client`.
//
// If you JUST want a single client and don't need streamclient.New to work,
// consider NewUnregisteredFake.
func NewFake() Fake {
	fakeRegistryMu.Lock()
	defer fakeRegistryMu.Unlock()

	id := fakeRegistryID + 1
	fakeRegistryID++

	ret := Fake{
		id,
		&fakeDialer{streams: map[types.StreamName]*fakeStream{}},
	}
	fakeRegistry[id] = ret
	return ret
}

// NewUnregisteredFake returns a new Fake as well as a *Client for this Fake.
//
// Fake.StreamServerPath() will return an empty string and Unregister is
// a no-op.
//
// Aside from the returned *Client there's no way to obtain another *Client
// for this Fake.
func NewUnregisteredFake(namespace types.StreamName) (Fake, *Client) {
	f := Fake{
		0,
		&fakeDialer{streams: map[types.StreamName]*fakeStream{}},
	}
	return f, &Client{f.dial, namespace}
}

// Fake implements an in-memory sink for Client connections and data.
//
// You can retrieve the collected data via the Data method.
type Fake struct {
	id   int64 // 0 means "unregistered"
	dial *fakeDialer
}

// Unregister idempotently removes this Fake from the global registry.
//
// This has no effect for a Fake created with NewUnregisteredFake.
//
// After calling Unregister, StreamServerPath will return an empty string.
func (f Fake) Unregister() {
	id := atomic.SwapInt64(&f.id, 0)
	if id == 0 {
		return
	}
	fakeRegistryMu.Lock()
	defer fakeRegistryMu.Unlock()
	delete(fakeRegistry, id)
}

// StreamServerPath returns a value suitable for streamclient.New that will
// allow construction of a *Client which points to this Fake.
//
// If this Fake was generated via NewUnregisteredFake, or you called
// Unregister on it, returns an empty string.
func (f Fake) StreamServerPath() string {
	id := atomic.LoadInt64(&f.id)
	if id == 0 {
		return ""
	}
	return fmt.Sprintf("fake:%d", id)
}

// FakeStreamData is data about a single stream for a client created with the
// "fake" protocol. You can retrieve this with Fake.Data().
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

// Data returns all absorbed data collected by this Fake so far.
//
// Note that the FakeStreamData objects are `live`, and so calling their methods
// will always get you the current value of those streams. You'll need to call
// this method again if you expect a new stream to show up, however.
func (f Fake) Data() map[types.StreamName]FakeStreamData {
	return f.dial.dumpFakeData()
}

// SetError causes New* methods for Clients of this Fake to return this error.
func (f Fake) SetError(err error) {
	f.dial.setFakeError(err)
}

func init() {
	protocolRegistry["fake"] = func(fakeId string) (dialer, error) {
		id, err := strconv.ParseInt(fakeId, 10, 64)
		if err != nil {
			return nil, err
		}

		fakeRegistryMu.Lock()
		defer fakeRegistryMu.Unlock()
		f, ok := fakeRegistry[id]
		if !ok {
			return nil, errors.Reason("unknown Fake %d", id).Err()
		}
		return f.dial, nil
	}
}
