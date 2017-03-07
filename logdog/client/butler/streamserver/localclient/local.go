// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package localclient

import (
	"io"

	"github.com/luci/luci-go/logdog/client/butler"
	"github.com/luci/luci-go/logdog/client/butlerlib/streamclient"
	"github.com/luci/luci-go/logdog/client/butlerlib/streamproto"
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
