// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coordinator

import (
	"github.com/luci/luci-go/common/api/logdog_coordinator/logs/v1"
	"github.com/luci/luci-go/common/auth"
	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/logdog/types"
	"github.com/luci/luci-go/common/prpc"
)

var (
	// Scopes is the set of scopes needed for the Coordinator user endpoints.
	Scopes = []string{
		auth.OAuthScopeEmail,
	}
)

// Client wraps a Logs client with user-friendly methods.
//
// Each method should operate independently, so calling methods from different
// goroutines must not cause any problems.
type Client struct {
	// C is the underlying LogsClient interface.
	C logdog.LogsClient

	project config.ProjectName
}

// NewClient returns a new Client instance bound to a pRPC Client.
func NewClient(c *prpc.Client, project config.ProjectName) *Client {
	return &Client{
		C:       logdog.NewLogsPRPCClient(c),
		project: project,
	}
}

// Stream returns a Stream instance for the named stream.
func (c *Client) Stream(path types.StreamPath) *Stream {
	return &Stream{
		c:    c,
		path: path,
	}
}
