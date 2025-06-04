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

package rdb

import (
	"context"
	"net/http"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/grpc/prpc"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/auth"
)

type RecorderClientFactory interface {
	MakeClient(ctx context.Context, host string) (*RecorderClient, error)
}

type prodClientFactory struct{}

// MakeClient implements RecorderClientFactory.
//
// Creates a client to interact with Recorder, acting as LUCI CV to access data.
func (pcf *prodClientFactory) MakeClient(ctx context.Context, host string) (*RecorderClient, error) {
	transport, err := auth.GetRPCTransport(ctx, auth.AsSelf)
	if err != nil {
		return nil, err
	}

	return &RecorderClient{
		client: rdbpb.NewRecorderPRPCClient(
			&prpc.Client{
				C:               &http.Client{Transport: transport},
				Host:            host,
				Options:         prpc.DefaultOptions(),
				MaxResponseSize: prpc.DefaultMaxResponseSize,
			}),
	}, nil
}

func NewRecorderClientFactory() RecorderClientFactory {
	return &prodClientFactory{}
}

type RecorderClient struct {
	client rdbpb.RecorderClient
}

func (rc *RecorderClient) MarkInvocationSubmitted(ctx context.Context, invocation string) error {
	req := &rdbpb.MarkInvocationSubmittedRequest{
		Invocation: invocation,
	}
	if _, err := rc.client.MarkInvocationSubmitted(ctx, req); err != nil {
		s, ok := appstatus.Get(err)
		if !ok {
			// no status code attached to the err
			return err
		}
		// Note: If it is already marked submitted the RPC will take no action, so
		// retrying these are safe.
		// PermissionDenied might indicate that either invocation cannot be found
		// or we do not have permission so we avoid retries.
		switch s.Code() {
		case codes.PermissionDenied, codes.InvalidArgument:
			return errors.Fmt("failed to mark %s submitted: %w", invocation, err)
		default:
			return transient.Tag.Apply(errors.Fmt("failed to mark %s submitted: %w", invocation, err))
		}
	}

	return nil
}
