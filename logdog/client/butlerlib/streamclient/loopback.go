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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/logdog/client/butler"
	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"
	"go.chromium.org/luci/logdog/common/types"
)

type localDialer struct {
	*butler.Butler
}

var _ dialer = localDialer{}

func (d localDialer) DialStream(forProcess bool, f streamproto.Flags) (io.WriteCloser, error) {
	r, w := io.Pipe()
	if err := d.AddStream(r, f.Descriptor()); err != nil {
		return nil, errors.Annotate(err, "adding stream").Err()
	}
	return w, nil
}

func (d localDialer) DialDgramStream(f streamproto.Flags) (DatagramStream, error) {
	r, w := io.Pipe()
	if err := d.AddStream(r, f.Descriptor()); err != nil {
		return nil, errors.Annotate(err, "adding stream").Err()
	}
	return &datagramStreamWriter{w}, nil
}

// NewLoopback makes a loopback Client attached to a Butler instance.
//
// NOTE: `ForProcess` will be a no-op for the created Client. If you intend to
// generate streams from this Client to attach to subprocesses, you would be
// better served by using New to create a real client.
func NewLoopback(b *butler.Butler, namespace types.StreamName) *Client {
	return &Client{localDialer{b}, namespace}
}
