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

	"github.com/luci/luci-go/common/auth"
	"github.com/luci/luci-go/common/cli"
)

func cmdCancel(defaultAuthOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `cancel [flags] <build id>`,
		ShortDesc: "cancel a build",
		LongDesc:  "Cancel a build.",
		CommandRun: func() subcommands.CommandRun {
			r := &cancelRun{}
			r.SetDefaultFlags(defaultAuthOpts)
			return r
		},
	}
}

type cancelRun struct {
	baseCommandRun
	buildIDArg
}

func (r *cancelRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)

	if err := r.parseArgs(args); err != nil {
		return r.done(ctx, err)
	}

	return r.callAndDone(ctx, "POST", fmt.Sprintf("builds/%d/cancel", r.buildID), nil)
}
