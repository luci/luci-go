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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/lhttp"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/lucictx"

	"go.chromium.org/luci/buildbucket"
	bbpb "go.chromium.org/luci/buildbucket/proto"
)

// BuildsClient is a trimmed version of `bbpb.BuildsClient` which only
// contains the required RPC methods for bbagent.
//
// The live implementation automatically binds the "x-buildbucket-token" key with
// a token where necessary.
//
// Note: The dummy implementation will always return an EMPTY Build message;
// Make sure any code using BuildsClient can handle this scenario.
type BuildsClient interface {
	UpdateBuild(ctx context.Context, in *bbpb.UpdateBuildRequest, opts ...grpc.CallOption) (*bbpb.Build, error)
}

var _ BuildsClient = dummyBBClient{}

var readMask = &bbpb.BuildMask{
	Fields: &fieldmaskpb.FieldMask{
		Paths: []string{
			"id",
			"cancel_time",
		},
	},
}

type dummyBBClient struct{}

func (dummyBBClient) UpdateBuild(ctx context.Context, in *bbpb.UpdateBuildRequest, opts ...grpc.CallOption) (*bbpb.Build, error) {
	return &bbpb.Build{}, nil
}

type liveBBClient struct {
	tok    string
	c      bbpb.BuildsClient
	retryF retry.Factory
}

func (bb *liveBBClient) UpdateBuild(ctx context.Context, in *bbpb.UpdateBuildRequest, opts ...grpc.CallOption) (build *bbpb.Build, err error) {
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(buildbucket.BuildbucketTokenHeader, bb.tok))
	err = retry.Retry(ctx, transient.Only(bb.retryF), func() (err error) {
		build, err = bb.c.UpdateBuild(ctx, in)

		// Attach transient tag to internal transient error and NotFound.
		code := status.Code(err)
		if grpcutil.IsTransientCode(code) || code == codes.NotFound || code == codes.DeadlineExceeded {
			err = transient.Tag.Apply(err)
		}
		return err
	}, func(err error, sleepTime time.Duration) {
		logging.Fields{
			logging.ErrorKey: err,
			"sleepTime":      sleepTime,
		}.Warningf(ctx, "UpdateBuild will retry in %s", sleepTime)
	})
	return
}

// Reads the build secrets from the environment and constructs a BuildsClient
// which can be used to update the build state.
//
// retryEnabled allows us to switch retries for this client on and off
func newBuildsClient(ctx context.Context, hostname string, retryF retry.Factory) (BuildsClient, *bbpb.BuildSecrets, error) {
	if hostname == "" {
		logging.Infof(ctx, "No buildbucket hostname set; making dummy buildbucket client.")
		return dummyBBClient{}, &bbpb.BuildSecrets{BuildToken: buildbucket.DummyBuildbucketToken}, nil
	}

	prpcClient := &prpc.Client{
		Host: hostname,
		Options: &prpc.Options{
			Insecure: lhttp.IsLocalHost(hostname),
			// As of 2021-06-28, the P99 of UpdateBuild latency is ~2.1s.
			// So 5s should be long enough for the most of requests.
			PerRPCTimeout: 5 * time.Second,
		},
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
		retryF,
	}, secrets, nil
}

// options for the dispatcher.Channel
func channelOpts(ctx context.Context) (*dispatcher.Options, <-chan error) {
	errorFn, errCh := dispatcher.ErrorFnReport(10, func(failedBatch *buffer.Batch, err error) bool {
		return transient.Tag.In(err)
	})
	opts := &dispatcher.Options{
		QPSLimit: rate.NewLimiter(rate.Every(3*time.Second), 1),
		MinQPS:   rate.Every(buildbucket.MinUpdateBuildInterval),
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
		DropFn:  dispatcher.DropFnSummarized(ctx, rate.NewLimiter(rate.Every(10*time.Second), 1)),
		ErrorFn: errorFn,
	}
	return opts, errCh
}

func mkSendFn(ctx context.Context, client BuildsClient, bID int64, canceledBuildCh *closeOnceCh) dispatcher.SendFn {
	return func(b *buffer.Batch) error {
		var req *bbpb.UpdateBuildRequest

		// Nil batch. Synthesize a UpdateBuild request.
		if b == nil {
			req = &bbpb.UpdateBuildRequest{
				Build: &bbpb.Build{
					Id: bID,
				},
				Mask: readMask,
			}
		} else if b.Meta != nil {
			req = b.Meta.(*bbpb.UpdateBuildRequest)
		} else {
			build := b.Data[0].Item.(*bbpb.Build)
			build.Tags = buildbucket.WithoutDisallowedTagKeys(build.Tags)
			req = &bbpb.UpdateBuildRequest{
				Build: build,
				UpdateMask: &field_mask.FieldMask{
					Paths: []string{
						"build.steps",
						"build.output",
						"build.summary_markdown",
					},
				},
				Mask: readMask,
			}
			if len(build.Tags) > 0 {
				req.UpdateMask.Paths = append(req.UpdateMask.Paths, "build.tags")
			}
			b.Meta = req
			b.Data[0].Item = nil
		}

		updatedBuild, err := client.UpdateBuild(ctx, req)
		if err != nil {
			return err
		}
		if updatedBuild.CancelTime != nil {
			logging.Infof(ctx, "The build is in the cancel process, cancel time is %s.", updatedBuild.CancelTime.AsTime().String())
			canceledBuildCh.close()
		}
		return nil
	}
}

// defaultRetryStrategy defines a default build client retry strategy in bbagent.
func defaultRetryStrategy() retry.Iterator {
	return &retry.ExponentialBackoff{
		Limited: retry.Limited{
			Delay:   200 * time.Millisecond,
			Retries: 10,
		},
		MaxDelay:   80 * time.Second,
		Multiplier: 2,
	}
}
