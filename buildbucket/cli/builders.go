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
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/system/pager"

	pb "go.chromium.org/luci/buildbucket/proto"
)

func cmdBuilders(p Params) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `builders [flags] [PATH]`,
		ShortDesc: "lists builders",
		LongDesc: doc(`
			Lists builders.

			A PATH argument has the form "<project>/<bucket>".
		`),
		CommandRun: func() subcommands.CommandRun {
			r := &buildersRun{}
			r.RegisterDefaultFlags(p)
			r.Flags.IntVar(&r.limit, "n", 0, doc(`
				Limit the number of builders to print. If 0, then unlimited.
			`))
			r.Flags.BoolVar(&r.noPager, "nopage", false, doc(`
				Disable paging.
			`))
			return r
		},
	}
}

type buildersRun struct {
	printRun

	limit   int
	noPager bool
}

func (r *buildersRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)

	if err := r.initClients(ctx, nil); err != nil {
		return r.done(ctx, err)
	}

	if r.limit < 0 {
		return r.done(ctx, fmt.Errorf("-n value must be non-negative"))
	}
	if len(args) != 1 {
		return r.done(ctx, fmt.Errorf("exactly one PATH of the form \"<project>/<bucket>\" must be specified"))
	}

	parts := strings.Split(args[0], "/")
	if len(parts) != 2 {
		return r.done(ctx, fmt.Errorf("PATH must have the form \"<project>/<bucket>\""))
	}

	req := &pb.ListBuildersRequest{
		Project: parts[0],
		Bucket:  parts[1],
	}

	disableColors := r.noColor || shouldDisableColors()

	listBuilders := func(ctx context.Context, out io.WriteCloser) int {
		p := newPrinter(out, disableColors, time.Now)

		for count := 0; count < r.limit || r.limit == 0; {
			if r.limit > 0 {
				req.PageSize = int32(r.limit - count)
			}
			if r.limit == 0 || req.PageSize > defaultPageSize {
				req.PageSize = defaultPageSize
			}
			rsp, err := r.buildersClient.ListBuilders(ctx, req, expectedCodeRPCOption)
			if err != nil {
				return r.done(ctx, err)
			}
			for _, b := range rsp.Builders {
				// TODO(crbug/1240349): Improve formatting when printing builders.
				p.f("%s/%s/%s\n", b.Id.Project, b.Id.Bucket, b.Id.Builder)
				count++
				if count > r.limit && r.limit > 0 {
					break
				}
			}
			if rsp.NextPageToken == "" {
				break
			}
			req.PageToken = rsp.NextPageToken
		}

		return 0
	}

	if r.noPager {
		return listBuilders(ctx, os.Stdout)
	}
	return pager.Main(ctx, listBuilders)
}
