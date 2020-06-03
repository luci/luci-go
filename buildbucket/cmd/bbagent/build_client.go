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

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/buildbucket"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/lhttp"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"
	"go.chromium.org/luci/grpc/prpc"
	"golang.org/x/time/rate"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/metadata"
)

// options for the dispatcher.Channel
func channelOpts(ctx context.Context) *dispatcher.Options {
	return &dispatcher.Options{
		QPSLimit: rate.NewLimiter(1, 1),
		Buffer: buffer.Options{
			BatchSize:    1,
			MaxLeases:    1,
			FullBehavior: &buffer.DropOldestBatch{MaxLiveItems: 1},
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
		ErrorFn: dispatcher.ErrorFnQuiet,
	}
}

func newBuildsClient(ctx context.Context, infraOpts *bbpb.BuildInfra_Buildbucket) (ret dispatcher.Channel, err error) {
	var sendFn dispatcher.SendFn
	if hostname := infraOpts.GetHostname(); hostname == "" {
		logging.Infof(ctx, "No buildbucket hostname set; making dummy buildbucket client.")
		sendFn = func(b *buffer.Batch) error {
			return nil // noop
		}
	} else {
		opts := prpc.DefaultOptions()
		opts.Insecure = lhttp.IsLocalHost(hostname)
		opts.Retry = nil // luciexe handles retries itself.

		prpcClient := &prpc.Client{
			Host:    hostname,
			Options: opts,
		}

		var secrets *bbpb.BuildSecrets
		secrets, err = readBuildSecrets(ctx)
		if err != nil {
			return
		}

		prpcClient.C, err = auth.NewAuthenticator(ctx, auth.SilentLogin, auth.Options{
			MonitorAs: "bbagent/buildbucket",
		}).Client()
		if err != nil {
			return
		}

		// TODO(iannucci): Exchange secret build token+nonce for a running build token
		// here to confirm that:
		//   * We're the ONLY ones servicing this build (detect duplicate Swarming
		//     tasks). Failure to exchange the token would let us know that we got
		//     double-booked.
		//   * Auth is properly configured for buildbucket before we start running the
		//     user code.
		sendFn = mkSendFn(ctx, secrets, bbpb.NewBuildsPRPCClient(prpcClient))
	}

	return dispatcher.NewChannel(ctx, channelOpts(ctx), sendFn)
}

func mkSendFn(ctx context.Context, secrets *bbpb.BuildSecrets, client bbpb.BuildsClient) dispatcher.SendFn {
	return func(b *buffer.Batch) error {
		ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(buildbucket.BuildTokenHeader, secrets.BuildToken))

		var req *bbpb.UpdateBuildRequest
		var final bool

		if b.Meta != nil {
			req = b.Meta.(*bbpb.UpdateBuildRequest)
			final = protoutil.IsEnded(req.Build.Status)
		} else {
			build := b.Data[0].(*bbpb.Build)
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
			final = protoutil.IsEnded(build.Status)
			if final {
				if build.Status != bbpb.Status_SUCCESS {
					req.UpdateMask.Paths = append(req.UpdateMask.Paths, "build.status")
				}
			}
			if len(build.Tags) > 0 {
				req.UpdateMask.Paths = append(req.UpdateMask.Paths, "build.tags")
			}
			b.Meta = req
			b.Data[0] = nil
		}

		var timeout time.Duration
		if final {
			timeout = 5 * time.Minute
		} else {
			// Scale the timeout by the number of steps present, bounding it between
			// 2s and 1m (only the final status gets > 1m timeout, which is probably
			// futile anyway, since this RPC is currently serviced by an AppEngine
			// frontend instance which is capped at a 60s request time).
			timeout = time.Duration(len(req.Build.GetSteps())) * (50 * time.Millisecond)
			if timeout < (2 * time.Second) {
				timeout = 2 * time.Second
			} else if timeout > time.Minute {
				timeout = time.Minute
			}
		}
		tctx, cancel := clock.WithTimeout(ctx, timeout)
		defer cancel()

		_, err := client.UpdateBuild(tctx, req)
		// TODO(iannucci): Always tag errors as transient for the 'final' build
		// update?
		return err
	}
}
