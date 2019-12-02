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
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"
	luciflag "go.chromium.org/luci/common/flag"

	"go.chromium.org/luci/resultdb/pbutil"
)

func cmdLs(p Params) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `ls [flags]`,
		ShortDesc: "query results",
		CommandRun: func() subcommands.CommandRun {
			r := &lsRun{}
			r.queryRun.registerFlags(p)
			r.Flags.Var(luciflag.StringSlice(&r.invIDs), "inv", text.Doc(`
				Retrieve results from the invocation with this ID.

				May be specified multiple times.
			`))

			// TODO(crbug.com/1021849): add flag -cl
			// TODO(crbug.com/1021849): add flag -var
			// TODO(crbug.com/1021849): add flag -watch

			return r
		},
	}
}

type lsRun struct {
	queryRun
	invIDs []string
}

func (r *lsRun) parseArgs(args []string) error {
	if len(r.invIDs) == 0 {
		return errors.Reason("-inv is required").Err()
	}

	if len(args) != 0 {
		return errors.Reason("unexpected positional arguments").Err()
	}

	return r.queryRun.validate()
}

func (r *lsRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)

	if err := r.parseArgs(args); err != nil {
		return r.done(err)
	}

	if err := r.initClients(ctx); err != nil {
		return r.done(err)
	}

	return r.done(r.queryAndPrint(ctx, r.InvocationNames()))
}

func (r *lsRun) InvocationNames() []string {
	names := make([]string, len(r.invIDs))
	for i, id := range r.invIDs {
		names[i] = pbutil.InvocationName(id)
	}
	return names
}
