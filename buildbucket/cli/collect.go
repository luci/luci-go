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

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"

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
				duration to wait between requests.

				Lower bound is 20s. Smaller values will be reset to 20s.
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

func (r *collectRun) checkBuildStatus(ctx context.Context, req *pb.GetBuildRequest) (*pb.Build, error) {
	getStatusReq := &pb.GetBuildStatusRequest{
		Id:          req.Id,
		Builder:     req.Builder,
		BuildNumber: req.BuildNumber,
	}
	return r.buildsClient.GetBuildStatus(ctx, getStatusReq)
}

func (r *collectRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)
	// bb collect should retry infinitely on transient or deadline_exceeded errors.
	if err := r.initClients(ctx, func() retry.Iterator {
		return &retry.ExponentialBackoff{
			Limited: retry.Limited{
				Delay:   1 * time.Second,
				Retries: -1, // infinite retry
			},
			MaxDelay:   10 * time.Minute,
			Multiplier: 2,
		}
	}); err != nil {
		return r.done(ctx, err)
	}

	fields, err := r.FieldMask()
	if err != nil {
		return r.done(ctx, err)
	}

	inverval := r.intervalArg
	if inverval < 20*time.Second {
		logging.Infof(ctx, "Got interval %s which is smaller than the lower bound 20s, reset it to 20s.", r.intervalArg)
		inverval = 20 * time.Second
	}

	return r.PrintAndDone(ctx, args, unordered, func(ctx context.Context, arg string) (*pb.Build, error) {
		req, err := protoutil.ParseGetBuildRequest(arg)
		if err != nil {
			return nil, err
		}
		req.Fields = fields

		for {
			build, err := r.checkBuildStatus(ctx, req)
			if err != nil {
				return nil, err
			}
			if build.Status&pb.Status_ENDED_MASK == 0 {
				logging.Infof(ctx, "build %d is still %s; sleeping for %s", build.Id, build.Status, inverval)
				time.Sleep(inverval)
			} else {
				return r.buildsClient.GetBuild(ctx, req)
			}
		}
	})
}
