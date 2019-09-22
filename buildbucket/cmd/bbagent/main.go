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

// Command bbagent is Buildbucket's agent running in swarming.
//
// This executable creates a luciexe 'host' environment, and runs the
// Buildbucket build's exe within this environment. Please see
// https://go.chromium.org/luci/luciexe for details about the 'luciexe'
// protocol.
//
// This command is an implementation detail of Buildbucket.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/golang/protobuf/proto"
	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/lhttp"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/logdog/client/butler/output"
	"go.chromium.org/luci/lucictx"
	"go.chromium.org/luci/luciexe/host"
	"go.chromium.org/luci/luciexe/invoke"
	"golang.org/x/time/rate"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/metadata"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

func main() {
	ctx := logging.SetLevel(gologger.StdConfig.Use(context.Background()), logging.Debug)

	check := func(err error) {
		if err != nil {
			logging.Errorf(ctx, err.Error())
			os.Exit(1)
		}
	}

	if len(os.Args) != 2 {
		check(errors.Reason("expected 1 argument after arg0, got %d", len(os.Args)-1).Err())
	}

	sctx, err := lucictx.SwitchLocalAccount(ctx, "system")
	check(errors.Annotate(err, "could not switch to 'system' account in LUCI_CONTEXT").Err())

	input := &bbpb.BBAgentArgs{}
	err = proto.Unmarshal([]byte(os.Args[1]), input)
	check(errors.Annotate(err, "could not unmarshal BBAgentArgs").Err())

	bbClient, err := newBuildsClient(sctx, input.Build.Infra.Buildbucket)
	check(errors.Annotate(err, "could not connect to Buildbucket").Err())
	defer bbClient.CloseAndDrain(ctx)

	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	opts := &host.Options{
		ViewerURL: fmt.Sprintf("https://ci.chromium.org/p/%d", input.Build.Id),
	}
	opts.LogdogOutput, err = mkLogdogOutput(sctx, input.Build.Infra.Logdog)
	check(err)
	opts.BaseDir, err = os.Getwd()
	check(errors.Annotate(err, "getting cwd").Err())

	builds, err := host.Run(cctx, opts, func(ctx context.Context) error {
		subp, err := invoke.Start(ctx, input.ExecutablePath, input.Build, &invoke.Options{
			CacheDir: input.CacheDir,
		})
		if err != nil {
			return err
		}
		_, err = subp.Wait()
		return err
	})
	if err != nil {
		check(errors.Annotate(err, "could not start luciexe host environment").Err())
	}
	for build := range builds {
		bbClient.C <- build
	}
}

func mkLogdogOutput(ctx context.Context, opts *bbpb.BuildInfra_LogDog) (output.Output, error) {
	return nil, nil
}

type buildUpdate func(context.Context, *bbpb.Build) error

func newBuildsClient(ctx context.Context, infraOpts *bbpb.BuildInfra_Buildbucket) (ret dispatcher.Channel, err error) {
	hostname := infraOpts.GetHostname()
	if hostname == "" {
		err = errors.New("missing hostname in build.infra.buildbucket")
		return
	}

	opts := prpc.DefaultOptions()
	opts.Insecure = lhttp.IsLocalHost(hostname)
	opts.Retry = nil // luciexe handles retries itself.

	secrets, err := readBuildSecrets(ctx)
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
	client := bbpb.NewBuildsPRPCClient(&prpc.Client{
		Host:    hostname,
		Options: opts,
	})

	return dispatcher.NewChannel(ctx, &dispatcher.Options{
		Buffer: buffer.Options{
			BatchDuration: time.Second,
			BatchSize:     1,
			MaxLeases:     1,
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
		QPSLimit: rate.NewLimiter(1, 1),
	}, func(b *buffer.Batch) error {
		ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(buildbucket.BuildTokenHeader, secrets.BuildToken))

		var req *bbpb.UpdateBuildRequest
		if b.Meta != nil {
			req = b.Meta.(*bbpb.UpdateBuildRequest)
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
			if protoutil.IsEnded(build.Status) {
				if build.Status != bbpb.Status_SUCCESS {
					req.UpdateMask.Paths = append(req.UpdateMask.Paths, "build.status")
				}
			}
			b.Meta = req
			b.Data[0] = nil
		}
		_, err := client.UpdateBuild(ctx, req)
		return err
	})
}

// readBuildSecrets reads BuildSecrets message from swarming secret bytes.
func readBuildSecrets(ctx context.Context) (*bbpb.BuildSecrets, error) {
	swarming := lucictx.GetSwarming(ctx)
	if swarming == nil {
		return nil, errors.Reason("no swarming secret bytes; is this a Swarming Task with secret bytes?").Err()
	}

	secrets := &bbpb.BuildSecrets{}
	if err := proto.Unmarshal(swarming.SecretBytes, secrets); err != nil {
		return nil, errors.Annotate(err, "failed to read BuildSecrets message from swarming secret bytes").Err()
	}
	return secrets, nil
}
