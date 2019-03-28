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
	"math/rand"
	"strconv"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/cli"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
)

func cmdAdd(defaultAuthOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `add [flags] builder [builder...]`,
		ShortDesc: "schedule builds",
		LongDesc: `Schedule builds.

Example:
	# Add linux-rel and mac-rel builds to chromium/ci bucket
	bb add infra/ci/{linux-rel,mac-rel}
`,
		CommandRun: func() subcommands.CommandRun {
			r := &addRun{}
			r.SetDefaultFlags(defaultAuthOpts)
			r.buildFieldFlags.Register(&r.Flags)
			// TODO(crbug.com/946422): add -cl flag
			// TODO(crbug.com/946422): add -commit flag
			return r
		},
	}
}

type addRun struct {
	baseCommandRun
	buildFieldFlags
}

func (r *addRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)

	req := &buildbucketpb.BatchRequest{}
	for _, a := range args {
		builder, err := protoutil.ParseBuilderID(a)
		if err != nil {
			return r.done(ctx, fmt.Errorf("invalid builder %q: %s", a, err))
		}
		req.Requests = append(req.Requests, &buildbucketpb.BatchRequest_Request{
			Request: &buildbucketpb.BatchRequest_Request_ScheduleBuild{
				ScheduleBuild: &buildbucketpb.ScheduleBuildRequest{
					RequestId: strconv.FormatInt(rand.Int63(), 10),
					Builder:   builder,
				},
			},
		})
	}

	return r.batchAndDone(ctx, req)
}
