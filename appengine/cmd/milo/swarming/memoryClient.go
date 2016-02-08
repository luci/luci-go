// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package swarming

import (
	"bytes"
	"fmt"

	"github.com/luci/luci-go/client/logdog/butlerlib/streamclient"
	"github.com/luci/luci-go/client/logdog/butlerlib/streamproto"
)

// In-memory datastructure to hold a fake butler client.
type memoryStream struct {
	*streamproto.Properties

	buf bytes.Buffer
	dg  []byte
}

func (s *memoryStream) Write(b []byte) (int, error) {
	return s.buf.Write(b)
}

func (s *memoryStream) Close() error {
	return nil
}

func (s *memoryStream) WriteDatagram(b []byte) error {
	s.dg = make([]byte, len(b))
	copy(s.dg, b)
	return nil
}

type memoryClient struct {
	stream map[string]*memoryStream
}

func (c *memoryClient) NewStream(f streamproto.Flags) (streamclient.Stream, error) {
	props := f.Properties()
	if _, ok := c.stream[props.Name]; ok {
		return nil, fmt.Errorf("duplicate stream, %q", props.Name)
	}
	s := memoryStream{
		Properties: props,
	}
	if c.stream == nil {
		c.stream = map[string]*memoryStream{}
	}
	c.stream[s.Name] = &s
	return &s, nil
}
