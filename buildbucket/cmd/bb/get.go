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
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/cli"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
)

func cmdGet(defaultAuthOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `get [flags] <build id>`,
		ShortDesc: "get details about a build",
		LongDesc:  "Get details about a build.",
		CommandRun: func() subcommands.CommandRun {
			r := &getRun{}
			r.SetDefaultFlags(defaultAuthOpts)
			r.buildFieldFlags.Register(&r.Flags)
			return r
		},
	}
}

type getRun struct {
	baseCommandRun
	buildIDArg
	buildFieldFlags
}

func (r *getRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)

	if err := r.parseArgs(args); err != nil {
		return r.done(ctx, err)
	}

	client, err := r.newClient(ctx)
	if err != nil {
		return r.done(ctx, err)
	}

	build, err := client.GetBuild(ctx, &buildbucketpb.GetBuildRequest{
		Id:     r.buildID,
		Fields: r.FieldMask(),
	})
	p := newStdoutPrinter()
	switch {
	case err != nil:
		return r.done(ctx, err)
	case r.json:
		p.JSONPB(build)
	default:
		p.Build(build)
	}
	return 0
}
