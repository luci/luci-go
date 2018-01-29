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
	"golang.org/x/net/context"

	"go.chromium.org/luci/common/auth"
	"go.chromium.org/luci/common/cli"
)

func cmdFmt(defaultAuthOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `fmt subcommand [arguments]`,
		ShortDesc: "converts a message to/from flagpb and JSON formats",
		LongDesc:  "Converts a message to/from flagpb and JSON formats.",
		CommandRun: func() subcommands.CommandRun {
			c := &fmtRun{defaultAuthOpts: defaultAuthOpts}
			return c
		},
	}
}

type fmtRun struct {
	cmdRun

	defaultAuthOpts auth.Options
}

func (r *fmtRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	app := &cli.Application{
		Name: "fmt",
		Context: func(context.Context) context.Context {
			return cli.GetContext(a, r, env)
		},
		Title: "Converts a message formats.",
		Commands: []*subcommands.Command{
			cmdJ2F(r.defaultAuthOpts),
			cmdF2J(r.defaultAuthOpts),
		},
	}
	return subcommands.Run(app, args)
}
