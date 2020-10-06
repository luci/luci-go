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
	"go.chromium.org/luci/buildbucket/cmd/bbagent/sink"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/lhttp"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/prpc"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/metadata"
)

func mkBuildBucketOutFn(ctx context.Context, infraOpts *bbpb.BuildInfra_Buildbucket) (sink.OutFn, error) {
	hostname := infraOpts.GetHostname()
	if hostname == "" {
		logging.Infof(ctx, "No buildbucket hostname set; making dummy BuildbucketOutFn.")
		return func(build *bbpb.Build) error { return nil }, nil
	}
	bc, err := newBuildsClient(ctx, hostname)
	if err != nil {
		return nil, errors.Annotate(err, "could not connect to buildbucket").Err()
	}
	secrets, err := readBuildSecrets(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "could not connect to buildbucket").Err()
	}

	return func(build *bbpb.Build) error {
		req := &bbpb.UpdateBuildRequest{
			Build: build,
			UpdateMask: &field_mask.FieldMask{
				Paths: []string{
					"build.summary_markdown",
				},
			},
		}
		final := protoutil.IsEnded(build.GetStatus())
		if final && build.Status != bbpb.Status_SUCCESS {
			req.UpdateMask.Paths = append(req.UpdateMask.Paths, "build.status")
		}
		if build.Output != nil {
			req.UpdateMask.Paths = append(req.UpdateMask.Paths, "build.output")
		}
		if len(build.Steps) > 0 {
			req.UpdateMask.Paths = append(req.UpdateMask.Paths, "build.steps")
		}
		if len(build.Tags) > 0 {
			req.UpdateMask.Paths = append(req.UpdateMask.Paths, "build.tags")
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
		octx := metadata.NewOutgoingContext(ctx, metadata.Pairs(buildbucket.BuildTokenHeader, secrets.BuildToken))
		tctx, cancel := clock.WithTimeout(octx, timeout)
		defer cancel()

		_, err := bc.UpdateBuild(tctx, req)
		// TODO(iannucci): Always tag errors as transient for the 'final' build
		// update?
		return err
	}, nil
}

func newBuildsClient(ctx context.Context, hostname string) (bbpb.BuildsClient, error) {
	opts := prpc.DefaultOptions()
	opts.Insecure = lhttp.IsLocalHost(hostname)
	opts.Retry = nil // luciexe handles retries itself.

	c, err := auth.NewAuthenticator(ctx, auth.SilentLogin, auth.Options{
		MonitorAs: "bbagent/buildbucket",
	}).Client()
	if err != nil {
		return nil, err
	}
	prpcClient := &prpc.Client{
		C:       c,
		Host:    hostname,
		Options: opts,
	}
	// TODO(iannucci): Exchange secret build token+nonce for a running build token
	// here to confirm that:
	//   * We're the ONLY ones servicing this build (detect duplicate Swarming
	//     tasks). Failure to exchange the token would let us know that we got
	//     double-booked.
	//   * Auth is properly configured for buildbucket before we start running the
	//     user code.
	return bbpb.NewBuildsPRPCClient(prpcClient), nil
}
