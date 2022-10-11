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
	"os"

	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"
)

// Similar to io.Discard, but also implements Close and WriteDatagram.
type nullWriterT int

var nullWriter interface {
	DatagramStream
	io.Writer
} = nullWriterT(0)

func (nullWriterT) Write(data []byte) (int, error) { return len(data), nil }
func (nullWriterT) WriteDatagram(dg []byte) error  { return nil }
func (nullWriterT) Close() error                   { return nil }

type nullDialerT int

var nullDialer dialer = (nullDialerT)(0)

func (nullDialerT) DialDgramStream(streamproto.Flags) (DatagramStream, error) { return nullWriter, nil }
func (nullDialerT) DialStream(forProcess bool, f streamproto.Flags) (io.WriteCloser, error) {
	if forProcess {
		return os.Create(os.DevNull)
	}
	return nullWriter, nil
}

func init() {
	protocolRegistry["null"] = func(string) (dialer, error) { return nullDialer, nil }
}
