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
	"time"

	"github.com/maruel/subcommands"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/grpcutil"

	pb "go.chromium.org/luci/buildbucket/proto"
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

func (r *collectRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)
	if err := r.initClients(ctx); err != nil {
		return r.done(ctx, err)
	}

	fields, err := r.FieldMask()
	if err != nil {
		return r.done(ctx, err)
	}

	return r.PrintAndDone(ctx, args, unordered, func(ctx context.Context, arg string) (*pb.Build, error) {
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
				logging.Infof(ctx, "build %d is still %s; sleeping for %s", build.Id, build.Status, r.intervalArg)
				time.Sleep(r.intervalArg)

			default:
				return build, nil
			}
		}
	})
}
