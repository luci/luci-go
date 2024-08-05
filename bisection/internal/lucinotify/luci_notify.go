// Copyright 2023 The LUCI Authors.
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

// Package lucinotify contains logic of interacting with LUCI Notify.
package lucinotify

import (
	"context"
	"net/http"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/prpc"
	lnpb "go.chromium.org/luci/luci_notify/api/service/v1"
	"go.chromium.org/luci/server/auth"
)

const (
	luciNotifyHost = "notify.api.luci.app"
)

// mockedLuciNotifyClientKey is the context key indicates using mocked LUCI Notify client in tests.
var mockedLUCINotifyClientKey = "used in tests only for setting the mock LUCI Notify client"

func newLUCINotifyClient(ctx context.Context, host string) (lnpb.TreeCloserClient, error) {
	if mockClient, ok := ctx.Value(&mockedLUCINotifyClientKey).(*lnpb.MockTreeCloserClient); ok {
		return mockClient, nil
	}

	t, err := auth.GetRPCTransport(ctx, auth.AsSelf)
	if err != nil {
		return nil, err
	}
	return lnpb.NewTreeCloserPRPCClient(
		&prpc.Client{
			C:       &http.Client{Transport: t},
			Host:    host,
			Options: prpc.DefaultOptions(),
		}), nil
}

// Client is the client to communicate with LUCI Notify.
type Client struct {
	Client lnpb.TreeCloserClient
}

// NewClient creates a client to communicate with LUCI Notify.
func NewClient(ctx context.Context, host string) (*Client, error) {
	client, err := newLUCINotifyClient(ctx, host)
	if err != nil {
		return nil, err
	}

	return &Client{
		Client: client,
	}, nil
}

func (c *Client) CheckTreeCloser(ctx context.Context, req *lnpb.CheckTreeCloserRequest) (*lnpb.CheckTreeCloserResponse, error) {
	return c.Client.CheckTreeCloser(ctx, req)
}

// CheckTreeCloser returns true if a builder (with failed step) is a tree closer.
func CheckTreeCloser(c context.Context, project string, bucket string, builder string, step string) (bool, error) {
	req := &lnpb.CheckTreeCloserRequest{
		Project: project,
		Bucket:  bucket,
		Builder: builder,
		Step:    step,
	}

	cl, err := NewClient(c, luciNotifyHost)
	if err != nil {
		return false, errors.Annotate(err, "couldn't create tree closer client").Err()
	}
	res, err := cl.CheckTreeCloser(c, req)
	if err != nil {
		return false, errors.Annotate(err, "check tree closer (%s, %s, %s, %s)", project, bucket, builder, step).Err()
	}

	return res.IsTreeCloser, nil
}
