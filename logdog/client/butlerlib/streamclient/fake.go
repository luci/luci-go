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

func init() {
	protocolRegistry["fake"] = func(addr string) (dialer, error) {
		if addr != "" {
			return nil, errors.New("fake protocol does not accept an address")
		}
		return &fakeDialer{streams: map[types.StreamName]*fakeStream{}}, nil
	}
}
