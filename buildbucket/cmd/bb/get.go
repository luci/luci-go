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
			r.SetDefaultFlags(defaultAuthOpts)
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

	if len(args) < 1 {
		return r.done(ctx, fmt.Errorf("missing an argument"))
	}
	if len(args) > 1 {
		return r.done(ctx, fmt.Errorf("unexpected arguments: %s", args[1:]))
	}

	req, err := protoutil.ParseGetBuildRequest(args[0])
	req.Fields = r.FieldMask()
	if err != nil {
		return r.done(ctx, err)
	}

	client, err := r.newClient(ctx)
	if err != nil {
		return r.done(ctx, err)
	}

	build, err := client.GetBuild(ctx, req)
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
