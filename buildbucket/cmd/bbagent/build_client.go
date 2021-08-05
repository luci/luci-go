// Copyright 2019 The LUCI Authors.
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

package main

import (
	"context"
	"time"

	"golang.org/x/time/rate"

	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/lhttp"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/lucictx"

	"go.chromium.org/luci/buildbucket"
	bbpb "go.chromium.org/luci/buildbucket/proto"
)

// BuildsClient is a trimmed version of `bbpb.BuildsClient` which only
// contains the required RPC methods for bbagent.
//
// The live implementation automatically binds the "X-Build-Token" key with
// a token where necessary.
type BuildsClient interface {
	GetBuild(ctx context.Context, in *bbpb.GetBuildRequest, opts ...grpc.CallOption) (*bbpb.Build, error)
	UpdateBuild(ctx context.Context, in *bbpb.UpdateBuildRequest, opts ...grpc.CallOption) (*bbpb.Build, error)
}

var _ BuildsClient = dummyBBClient{}

type dummyBBClient struct{}

func (dummyBBClient) GetBuild(ctx context.Context, in *bbpb.GetBuildRequest, opts ...grpc.CallOption) (*bbpb.Build, error) {
	return nil, nil
}

func (dummyBBClient) UpdateBuild(ctx context.Context, in *bbpb.UpdateBuildRequest, opts ...grpc.CallOption) (*bbpb.Build, error) {
	return nil, nil
}

type liveBBClient struct {
	tok string
	c   bbpb.BuildsClient
}

func (bb *liveBBClient) GetBuild(ctx context.Context, in *bbpb.GetBuildRequest, opts ...grpc.CallOption) (*bbpb.Build, error) {
	return bb.c.GetBuild(ctx, in, opts...)
}

func (bb *liveBBClient) UpdateBuild(ctx context.Context, in *bbpb.UpdateBuildRequest, opts ...grpc.CallOption) (*bbpb.Build, error) {
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(buildbucket.BuildTokenHeader, bb.tok))
	return bb.c.UpdateBuild(ctx, in, opts...)
}

// Reads the build secrets from the environment and constructs a BuildsClient
// which can be used to update the build state.
//
// retryEnabled allows us to switch retries for this client on and off
func newBuildsClient(ctx context.Context, hostname string, retryEnabled *bool) (BuildsClient, *bbpb.BuildSecrets, error) {
	if hostname == "" {
		logging.Infof(ctx, "No buildbucket hostname set; making dummy buildbucket client.")
		return dummyBBClient{}, &bbpb.BuildSecrets{BuildToken: "dummy token"}, nil
	}
	opts := prpc.DefaultOptions()
	opts.Insecure = lhttp.IsLocalHost(hostname)
	originalRetry := opts.Retry
	opts.Retry = func() retry.Iterator {
		if *retryEnabled {
			return originalRetry()
		}
		return nil
	}

	// As of 2021-06-28, the P99 of UpdateBuild latency is ~2.1s.
	// So 5s should be long enough for the most of requests.
	opts.PerRPCTimeout = 5 * time.Second

	prpcClient := &prpc.Client{
		Host:    hostname,
		Options: opts,
	}
	secrets, err := readBuildSecrets(ctx)
	if err != nil {
		return nil, nil, err
	}

	// Use "system" account to call UpdateBuild RPCs.
	sctx, err := lucictx.SwitchLocalAccount(ctx, "system")
	if err != nil {
		return nil, nil, errors.Annotate(err, "could not switch to 'system' account in LUCI_CONTEXT").Err()
	}
	prpcClient.C, err = auth.NewAuthenticator(sctx, auth.SilentLogin, auth.Options{
		MonitorAs: "bbagent/buildbucket",
	}).Client()
	if err != nil {
		return nil, nil, err
	}
	// TODO(iannucci): Exchange secret build token+nonce for a running build token
	// here to confirm that:
	//   * We're the ONLY ones servicing this build (detect duplicate Swarming
	//     tasks). Failure to exchange the token would let us know that we got
	//     double-booked.
	//   * Auth is properly configured for buildbucket before we start running the
	//     user code.
	return &liveBBClient{
		secrets.BuildToken,
		bbpb.NewBuildsPRPCClient(prpcClient),
	}, secrets, nil
}

// options for the dispatcher.Channel
func channelOpts(ctx context.Context) (*dispatcher.Options, <-chan error) {
	errorFn, errCh := dispatcher.ErrorFnReport(10, func(failedBatch *buffer.Batch, err error) bool {
		return transient.Tag.In(err)
	})
	opts := &dispatcher.Options{
		QPSLimit: rate.NewLimiter(1, 1),
		Buffer: buffer.Options{
			MaxLeases:     1,
			BatchItemsMax: 1,
			FullBehavior:  &buffer.DropOldestBatch{MaxLiveItems: 1},
			Retry: func() retry.Iterator {
				return &retry.ExponentialBackoff{
					Limited: retry.Limited{
						Delay:    200 * time.Millisecond, // initial delay
						Retries:  -1,
						MaxTotal: 5 * time.Minute,
					},
					Multiplier: 1.2,
					MaxDelay:   30 * time.Second,
				}
			},
		},
		DropFn:  dispatcher.DropFnSummarized(ctx, rate.NewLimiter(.1, 1)),
		ErrorFn: errorFn,
	}
	return opts, errCh
}

func mkSendFn(ctx context.Context, client BuildsClient) dispatcher.SendFn {
	return func(b *buffer.Batch) error {
		var req *bbpb.UpdateBuildRequest

		if b.Meta != nil {
			req = b.Meta.(*bbpb.UpdateBuildRequest)
		} else {
			build := b.Data[0].Item.(*bbpb.Build)
			buildbucket.StripDisallowedTagKeys(&build.Tags)
			req = &bbpb.UpdateBuildRequest{
				Build: build,
				UpdateMask: &field_mask.FieldMask{
					Paths: []string{
						"build.steps",
						"build.output",
						"build.summary_markdown",
					},
				},
			}
			if len(build.Tags) > 0 {
				req.UpdateMask.Paths = append(req.UpdateMask.Paths, "build.tags")
			}
			b.Meta = req
			b.Data[0].Item = nil
		}

		_, err := client.UpdateBuild(ctx, req)
		return err
	}
}
