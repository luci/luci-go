// Copyright 2016 The LUCI Authors.
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

package localclient

import (
	"io"

	"go.chromium.org/luci/logdog/client/butler"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"
)

// localClient is a Client implementation that is directly bound to a Butler
// instance.
type localClient struct {
	*butler.Butler
}

var _ streamclient.Client = (*localClient)(nil)

// New creates a new Client instance bound to a local Butler instance. This
// sidesteps the need for an actual server socket and negotiation protocol and
// directly registers streams via Butler API calls.
//
// Internally, the Stream uses an io.PipeWriter and io.PipeReader to ferry
// data from the Stream's owner to the Butler.
func New(b *butler.Butler) streamclient.Client {
	return &localClient{b}
}

func (c *localClient) NewStream(f streamproto.Flags) (s streamclient.Stream, err error) {
	pr, pw := io.Pipe()
	defer func() {
		if err != nil {
			pr.Close()
			pw.Close()
		}
	}()

	props := f.Properties()
	stream := streamclient.BaseStream{
		WriteCloser: pw,
		P:           props,
	}

	// Add the Stream to the Butler.
	if err = c.AddStream(pr, props); err != nil {
		return
	}
	return &stream, nil
}
