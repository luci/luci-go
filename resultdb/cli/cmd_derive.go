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
)

func cmdDerive(p Params) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `derive [flags] SWARMING_HOST TASK_ID [TASK_ID]...`,
		ShortDesc: "derive results from Chromium swarming tasks and query them",
		LongDesc: text.Doc(`
			Derives Invocation(s) from Chromium Swarming task(s) and prints results,
			like ls subcommand.
			If an invocation already exists for a given task, then reuses it.

			SWARMING_HOST must be a hostname without a scheme, e.g.
			chromium-swarm.appspot.com.
		`),
		CommandRun: func() subcommands.CommandRun {
			r := &deriveRun{}
			r.queryRun.registerFlags(p)
			return r
		},
	}
}

type deriveRun struct {
	queryRun
}

func (r *deriveRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)

	if err := r.queryRun.validate(); err != nil {
		return r.done(err)
	}

	if err := r.initClients(ctx); err != nil {
		return r.done(err)
	}

	// TODO(crbug.com/1021849): parse arguments.
	// TODO(crbug.com/1021849): derive invocations and call queryAndPrint.
	panic("unimplemented")
}
