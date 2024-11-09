// Copyright 2015 The LUCI Authors.
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

package coordinator

import (
	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/grpc/prpc"

	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/logs/v1"
	"go.chromium.org/luci/logdog/common/types"
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
	// Host is the LogDog host. This is loaded from the pRPC client in NewClient.
	Host string
}

// NewClient returns a new Client instance bound to a pRPC Client.
func NewClient(c *prpc.Client) *Client {
	return &Client{
		C:    logdog.NewLogsPRPCClient(c),
		Host: c.Host,
	}
}

// Stream returns a Stream instance for the named stream.
func (c *Client) Stream(project string, path types.StreamPath) *Stream {
	return &Stream{
		c:       c,
		project: project,
		path:    path,
	}
}
