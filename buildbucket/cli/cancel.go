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
	"fmt"
	"strings"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"

	pb "go.chromium.org/luci/buildbucket/proto"
)

func cmdCancel(p Params) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `cancel [flags] [BUILD [BUILD...]]`,
		ShortDesc: "cancel builds",
		LongDesc: doc(`
			Cancel builds.

			Argument BUILD can be an int64 build id or a string
			<project>/<bucket>/<builder>/<build_number>, e.g.
			chromium/ci/linux-rel/1.
			If no builds were specified on the command line, they are read
			from stdin.
		`),
		CommandRun: func() subcommands.CommandRun {
			r := &cancelRun{}
			r.RegisterDefaultFlags(p)
			r.RegisterFieldFlags()
			r.Flags.StringVar(&r.reason, "reason", "", doc(`
				reason of cancelation in Markdown format; required
			`))
			return r
		},
	}
}

type cancelRun struct {
	printRun
	reason string
}

func (r *cancelRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)
	if err := r.initClients(ctx); err != nil {
		return r.done(ctx, err)
	}

	r.reason = strings.TrimSpace(r.reason)
	if r.reason == "" {
		return r.done(ctx, fmt.Errorf("-reason is required"))
	}

	fields, err := r.FieldMask()
	if err != nil {
		return r.done(ctx, err)
	}

	return r.PrintAndDone(ctx, args, argOrder, func(ctx context.Context, arg string) (*pb.Build, error) {
		id, err := r.retrieveBuildID(ctx, arg)
		if err != nil {
			return nil, err
		}

		req := &pb.CancelBuildRequest{
			Id:              id,
			SummaryMarkdown: r.reason,
			Fields:          fields,
		}
		return r.client.CancelBuild(ctx, req, expectedCodeRPCOption)
	})
}
