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

	"github.com/golang/protobuf/proto"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/flag"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
)

func cmdPut(defaultAuthOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `put [flags] builder [builder...]`,
		ShortDesc: "schedule builds",
		LongDesc: `Schedule builds.

Example:
	bb put infra/try/linux-rel infra/try/mac-rel

Creates linux-rel and mac-rel builds in chromium/try bucket.
`,
		CommandRun: func() subcommands.CommandRun {
			r := &putRun{}
			r.SetDefaultFlags(defaultAuthOpts)
			r.buildFieldFlags.Register(&r.Flags)
			r.Flags.Var(flag.StringSlice(&r.cls), "cl", "CL input for the builds. Can be specified multiple times.")
			// TODO(crbug.com/946422): add -commit flag
			return r
		},
	}
}

type putRun struct {
	baseCommandRun
	buildFieldFlags
	cls []string
}

func (r *putRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)

	baseReq := &buildbucketpb.ScheduleBuildRequest{
		RequestId: strconv.FormatInt(rand.Int63(), 10),
	}
	for _, cl := range r.cls {
		change, err := r.retrieveCL(ctx, cl)
		if err != nil {
			return r.done(ctx, fmt.Errorf("parsing CL %q: %s", cl, err))
		}
		baseReq.GerritChanges = append(baseReq.GerritChanges, change)
	}

	req := &buildbucketpb.BatchRequest{}
	for i, a := range args {
		schedReq := proto.Clone(baseReq).(*buildbucketpb.ScheduleBuildRequest)
		schedReq.RequestId += fmt.Sprintf("-%d", i)

		var err error
		schedReq.Builder, err = protoutil.ParseBuilderID(a)
		if err != nil {
			return r.done(ctx, fmt.Errorf("invalid builder %q: %s", a, err))
		}
		req.Requests = append(req.Requests, &buildbucketpb.BatchRequest_Request{
			Request: &buildbucketpb.BatchRequest_Request_ScheduleBuild{ScheduleBuild: schedReq},
		})
	}

	return r.batchAndDone(ctx, req)
}
