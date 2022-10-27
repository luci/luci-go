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

// Package gerrit contains logic for interacting with Gerrit
package gerrit

import (
	"context"
	"fmt"
	"net/http"

	"go.chromium.org/luci/common/api/gerrit"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/server/auth"
)

// TODO (aredulla): check if Gerrit actions are enabled in config settings
// for each action

// mockedGerritClientKey is the context key to indicate using mocked
// Gerrit client in tests
var mockedGerritClientKey = "mock Gerrit client"

// Client is the client to communicate with Gerrit
// It wraps a gerritpb.GerritClient
type Client struct {
	gerritClient gerritpb.GerritClient
	host         string
}

func newGerritClient(ctx context.Context, host string) (gerritpb.GerritClient, error) {
	if mockClient, ok := ctx.Value(&mockedGerritClientKey).(*gerritpb.MockGerritClient); ok {
		// return a mock Gerrit client for tests
		return mockClient, nil
	}

	t, err := auth.GetRPCTransport(ctx, auth.AsSelf, auth.WithScopes(gerrit.OAuthScope))
	if err != nil {
		return nil, err
	}

	return gerrit.NewRESTClient(&http.Client{Transport: t}, host, true)
}

// NewClient creates a client to communicate with Gerrit
func NewClient(ctx context.Context, host string) (*Client, error) {
	client, err := newGerritClient(ctx, host)
	if err != nil {
		return nil, err
	}

	return &Client{
		gerritClient: client,
		host:         host,
	}, nil
}

// GetChange gets the corresponding change info given the commit ID.
// This function returns an error if none or more than 1 changes are returned
// by Gerrit.
func (c *Client) GetChange(ctx context.Context, commitID string) (*gerritpb.ChangeInfo, error) {
	req := &gerritpb.ListChangesRequest{
		Query: fmt.Sprintf("commit:\"%s\"", commitID),
		Options: []gerritpb.QueryOption{
			gerritpb.QueryOption_LABELS,
			gerritpb.QueryOption_MESSAGES,
			gerritpb.QueryOption_CHANGE_ACTIONS,
			gerritpb.QueryOption_SKIP_MERGEABLE,
			gerritpb.QueryOption_CHECK,
		},
	}

	res, err := c.gerritClient.ListChanges(ctx, req)
	if err != nil {
		logging.Errorf(ctx, "error getting change from Gerrit host %s for commit %s",
			c.host, commitID,
		)
		return nil, err
	}

	if len(res.Changes) == 0 {
		logging.Errorf(ctx, "no change found from Gerrit host %s for commit %s",
			c.host, commitID,
		)
		return nil, fmt.Errorf("no change found from Gerrit host %s for commit %s",
			c.host, commitID,
		)
	}

	if len(res.Changes) > 1 {
		logging.Errorf(ctx, "multiple changes found from Gerrit host %s for commit %s",
			c.host, commitID,
		)
		return nil, fmt.Errorf("multiple changes found from Gerrit host %s for commit %s",
			c.host, commitID,
		)
	}

	return res.Changes[0], nil
}
