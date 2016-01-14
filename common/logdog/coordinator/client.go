// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coordinator

import (
	"encoding/base64"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/common/api/logdog_coordinator/logs/v1"
	"github.com/luci/luci-go/common/logdog/types"
)

// Client is an abstract coordinator service implementation.
//
// Each method should operate independently, so calling methods from different
// goroutines must not cause any problems.
type Client struct {
	c   *Config
	svc *logs.Service
}

// New instantiates a new Client.
func New(conf Config) (*Client, error) {
	svc, err := conf.logs()
	if err != nil {
		return nil, err
	}

	return &Client{
		c:   &conf,
		svc: svc,
	}, nil
}

// Stream returns a Stream instance for the named stream.
func (c *Client) Stream(path types.StreamPath) *Stream {
	return &Stream{
		c:    c,
		path: path,
	}
}

func b64Decode(s string, m proto.Message) error {
	d, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return err
	}

	if err := proto.Unmarshal(d, m); err != nil {
		return err
	}

	return nil
}
