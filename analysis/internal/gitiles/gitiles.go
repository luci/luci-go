// Copyright 2024 The LUCI Authors.
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

// Package gitiles contains logic for interacting with gitiles.
package gitiles

import (
	"context"
	"net/http"
	"regexp"

	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/errors"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/server/auth"
)

// testingGitilesClientKey is the context key to indicate using a substitute
// Gitiles client in tests.
var testingGitilesClientKey = "used in tests only for setting the mock gitiles client"

// Client is the client to communicate with Gitiles.
// It wraps a gitilespb.GitilesClient.
type Client struct {
	gitilesClient gitilespb.GitilesClient
}

func newGitilesClient(ctx context.Context, host string, as auth.RPCAuthorityKind) (gitilespb.GitilesClient, error) {
	if testingClient, ok := ctx.Value(&testingGitilesClientKey).(FakeClient); ok {
		// return a Gitiles client for tests.
		return &testingClient, nil
	}

	t, err := auth.GetRPCTransport(ctx, as)
	if err != nil {
		return nil, err
	}
	return gitiles.NewRESTClient(&http.Client{Transport: t}, host, false)
}

func NewClient(ctx context.Context, host string, as auth.RPCAuthorityKind) (*Client, error) {
	if err := validHostname(host); err != nil {
		return nil, err
	}
	gitilesClient, err := newGitilesClient(ctx, host, as)
	if err != nil {
		return nil, errors.Annotate(err, "creating Gitiles client for host %s", host).Err()
	}
	return &Client{
		gitilesClient: gitilesClient,
	}, nil
}

func (c *Client) Log(ctx context.Context, req *gitilespb.LogRequest) (*gitilespb.LogResponse, error) {
	return c.gitilesClient.Log(ctx, req)
}

func (c *Client) DownloadFile(ctx context.Context, req *gitilespb.DownloadFileRequest) (*gitilespb.DownloadFileResponse, error) {
	return c.gitilesClient.DownloadFile(ctx, req)
}

var hostnameRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])*.googlesource.com$`)

func validHostname(hostname string) error {
	if !hostnameRe.MatchString(hostname) {
		return errors.Reason("hostname %s doesn't match %s", hostname, hostnameRe).Err()
	}
	return nil
}
