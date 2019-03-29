// Copyright 2016 The LUCI Authors.
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
	"fmt"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/cli"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
)

func cmdCancel(defaultAuthOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `cancel [flags] <BUILD> [<BUILD>...]`,
		ShortDesc: "cancel builds",
		LongDesc: `Cancel builds.

Argument BUILD can be an int64 build id or a string
<project>/<bucket>/<builder>/<build_number>, e.g. chromium/ci/linux-rel/1

-summary is required and it should explain the reason of cancelation.

If -json is true, then the stdout is a sequence of JSON objects representing
buildbucket.v2.Build protobuf messages. Not an array.`,
		CommandRun: func() subcommands.CommandRun {
			r := &cancelRun{}
			r.buildFieldFlags.Register(&r.Flags)
			r.RegisterGlobalFlags(defaultAuthOpts)
			r.Flags.StringVar(&r.summary, "summary", "", "reason of cancelation; required")
			return r
		},
	}
}

type cancelRun struct {
	baseCommandRun
	buildFieldFlags
	summary string
}

func (r *cancelRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)

	if r.summary == "" {
		return r.done(ctx, fmt.Errorf("-summary is required"))
	}

	buildIDs, err := r.retrieveBuildIDs(ctx, args)
	if err != nil {
		return r.done(ctx, err)
	}

	req := &buildbucketpb.BatchRequest{}
	fields := r.FieldMask()
	for _, id := range buildIDs {
		req.Requests = append(req.Requests, &buildbucketpb.BatchRequest_Request{
			Request: &buildbucketpb.BatchRequest_Request_CancelBuild{
				CancelBuild: &buildbucketpb.CancelBuildRequest{
					Id:              id,
					SummaryMarkdown: r.summary,
					Fields:          fields,
				},
			},
		})
	}

	return r.batchAndDone(ctx, req)
}
