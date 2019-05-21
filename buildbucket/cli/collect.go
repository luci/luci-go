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

package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/maruel/subcommands"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/grpcutil"

	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

func cmdCollect(p Params) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `collect [flags] <BUILD> [<BUILD>...]`,
		ShortDesc: "wait for builds to end",
		LongDesc: doc(`
			Wait for builds to end.

			Argument BUILD can be an int64 build id or a string
			<project>/<bucket>/<builder>/<build_number>, e.g. chromium/ci/linux-rel/1

			Optionally writes build details into an output file as JSON array of
			bulidbucket.v2.Build proto messages.
		`),
		CommandRun: func() subcommands.CommandRun {
			r := &collectRun{}
			r.RegisterDefaultFlags(p)
			r.RegisterFieldFlags()
			r.Flags.DurationVar(&r.intervalArg, "interval", time.Minute, doc(`
				duration to wait between requests
			`))
			return r
		},
		Advanced: true,
	}
}

type collectRun struct {
	printRun
	intervalArg time.Duration
}

func (r *collectRun) collectBuildDetails(ctx context.Context, client pb.BuildsClient, buildIDs map[int64]struct{}, sleep func()) error {
	stdout, stderr := newStdioPrinters(r.noColor)

	fm, err := r.FieldMask()
	if err != nil {
		return err
	}

	ended := make(map[int64]*pb.Build, len(buildIDs))
	printedBuild := false
	for {
		breq := &pb.BatchRequest{}
		for id := range buildIDs {
			if _, ok := ended[id]; ok {
				continue
			}

			breq.Requests = append(breq.Requests, &pb.BatchRequest_Request{
				Request: &pb.BatchRequest_Request_GetBuild{
					GetBuild: &pb.GetBuildRequest{
						Id:     id,
						Fields: fm,
					},
				},
			})
		}

		bresp, err := client.Batch(ctx, breq)
		if err != nil {
			return err
		}

		for _, resp := range bresp.Responses {
			error := resp.GetError()
			code := codes.Code(error.GetCode())
			build := resp.GetGetBuild()
			switch {
			case grpcutil.IsTransientCode(code):
				logging.Warningf(ctx, "transient %s error: %s", code, error.Message)

			case code != codes.OK:
				return fmt.Errorf("RPC error %s: %s", code, error.Message)

			case build.Status&pb.Status_ENDED_MASK != 0:
				ended[build.Id] = build
				r.printBuild(stdout, build, !printedBuild)
				printedBuild = true
			}
		}

		if len(buildIDs) == len(ended) {
			break
		}

		stderr.f("%d are still running/pending; sleeping...", len(buildIDs)-len(ended))
		sleep()
	}
	return nil
}

func (r *collectRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)
	if err := r.initClients(ctx); err != nil {
		return r.done(ctx, err)
	}

	fields, err := r.FieldMask()
	if err != nil {
		return r.done(ctx, err)
	}

	return r.PrintAndDone(ctx, args, false, func(ctx context.Context, arg string) (*pb.Build, error) {
		req, err := protoutil.ParseGetBuildRequest(arg)
		if err != nil {
			return nil, err
		}
		req.Fields = fields

		for {
			build, err := r.client.GetBuild(ctx, req)
			switch code := grpcutil.Code(err); {
			case grpcutil.IsTransientCode(code):
				logging.Warningf(ctx, "transient error: %s", err)

			case code != codes.OK:
				return nil, err

			case build.Status&pb.Status_ENDED_MASK == 0:
				// The build did not complete yet.
				time.Sleep(r.intervalArg)

			default:
				return build, nil
			}
		}
	})
}
