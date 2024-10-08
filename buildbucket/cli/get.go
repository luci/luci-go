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

package cli

import (
	"context"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"

	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

func cmdGet(p Params) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `get [flags] [BUILD [BUILD...]]`,
		ShortDesc: "print builds",
		LongDesc: doc(`
			Print builds.

			Argument BUILD can be an int64 build id or a string
			<project>/<bucket>/<builder>/<build_number>, e.g.
			chromium/ci/linux-rel/1.
			If no builds were specified on the command line, they are read
			from stdin.
		`),
		CommandRun: func() subcommands.CommandRun {
			r := &getRun{}
			r.RegisterDefaultFlags(p)
			r.RegisterFieldFlags()
			return r
		},
	}
}

type getRun struct {
	printRun
}

func (r *getRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)
	if err := r.initClients(ctx, nil); err != nil {
		return r.done(ctx, err)
	}

	fields, err := r.FieldMask()
	if err != nil {
		return r.done(ctx, err)
	}

	return r.PrintAndDone(ctx, args, argOrder, func(ctx context.Context, arg string) (*pb.Build, error) {
		req, err := protoutil.ParseGetBuildRequest(arg)
		if err != nil {
			return nil, err
		}
		req.Fields = fields
		return r.buildsClient.GetBuild(ctx, req, expectedCodeRPCOption)
	})
}
