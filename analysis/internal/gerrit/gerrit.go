// Copyright 2022 The LUCI Authors.
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

// Package gerrit contains logic for interacting with Gerrit.
package gerrit

import (
	"context"
	"net/http"

	"go.chromium.org/luci/common/api/gerrit"
	"go.chromium.org/luci/common/errors"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/analysis/internal/scopedauth"
)

// testingGerritClientKey is the context key to indicate using a substitute
// Gerrit client in tests.
var testingGerritClientKey = "used in tests only for setting the mock gerrit client"

// gerritClientFactory generates gerrit clients for testing that are specific to
// the host requested.
type gerritClientFactory interface {
	WithHost(host string) gerritpb.GerritClient
}

// Client is the client to communicate with Gerrit.
// It wraps a gerritpb.GerritClient.
type Client struct {
	gerritClient gerritpb.GerritClient
}

func newGerritClient(ctx context.Context, host, project string) (gerritpb.GerritClient, error) {
	if testingClientFactory, ok := ctx.Value(&testingGerritClientKey).(gerritClientFactory); ok {
		// return a Gerrit client for tests.
		return testingClientFactory.WithHost(host), nil
	}

	t, err := scopedauth.GetRPCTransport(ctx, project, auth.WithScopes(gerrit.OAuthScope))
	if err != nil {
		return nil, err
	}

	return gerrit.NewRESTClient(&http.Client{Transport: t}, host, true)
}

// NewClient creates a client to communicate with Gerrit, acting as the
// given LUCI Project.
func NewClient(ctx context.Context, host, project string) (*Client, error) {
	client, err := newGerritClient(ctx, host, project)
	if err != nil {
		return nil, errors.Fmt("creating Gerrit client for host %s: %w", host, err)
	}

	return &Client{
		gerritClient: client,
	}, nil
}

// GetChange gets a gerrit change by its ID.
func (c *Client) GetChange(ctx context.Context, req *gerritpb.GetChangeRequest) (*gerritpb.ChangeInfo, error) {
	res, err := c.gerritClient.GetChange(ctx, req)
	if err != nil {
		return nil, err
	}
	return res, nil
}
