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
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/cli"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
)

func cmdGet(defaultAuthOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `get [flags] <BUILD>`,
		ShortDesc: "get details about a build",
		LongDesc: `Get details about a build.

Argument BUILD can be an int64 build id or a string
<project>/<bucket>/<builder>/<build_number>, e.g. chromium/ci/linux-rel/1
`,
		CommandRun: func() subcommands.CommandRun {
			r := &getRun{}
			r.RegisterGlobalFlags(defaultAuthOpts)
			r.buildFieldFlags.Register(&r.Flags)
			return r
		},
	}
}

type getRun struct {
	baseCommandRun
	buildFieldFlags
}

func (r *getRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)
	if err := r.initClients(ctx); err != nil {
		return r.done(ctx, err)
	}

	req := &buildbucketpb.BatchRequest{}
	fields := r.FieldMask()
	for _, a := range args {
		getBuild, err := protoutil.ParseGetBuildRequest(a)
		if err != nil {
			return r.done(ctx, fmt.Errorf("invalid build %q: %s", a, err))
		}
		getBuild.Fields = fields
		req.Requests = append(req.Requests, &buildbucketpb.BatchRequest_Request{
			Request: &buildbucketpb.BatchRequest_Request_GetBuild{GetBuild: getBuild},
		})
	}

	return r.batchAndDone(ctx, req)
}
