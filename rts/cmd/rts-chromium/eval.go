// Copyright 2020 The LUCI Authors.
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
	"context"
	"os"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/rts/presubmit/eval"
)

func cmdEval() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `eval`,
		ShortDesc: "evaluate Chromium's test selection strategy",
		LongDesc:  "Evaluate Chromium's test selection strategy",
		CommandRun: func() subcommands.CommandRun {
			r := &evalRun{}
			if err := r.ev.RegisterFlags(&r.Flags); err != nil {
				panic(err) // should never happen
			}
			r.graph.RegisterFlags(&r.Flags)
			// TODO(nodir): add -fg-sibling-relevance flag.
			return r
		},
	}
}

type evalRun struct {
	baseCommandRun
	graph graphLoader
	ev    eval.CLI
}

func (r *evalRun) validateFlags(args []string) error {
	if err := r.ev.ValidateFlags(); err != nil {
		return err
	}
	if err := r.graph.ValidateFlags(); err != nil {
		return err
	}
	if len(args) > 0 {
		return errors.New("unexpected positional arguments")
	}
	return nil
}

func (r *evalRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)
	if err := r.validateFlags(args); err != nil {
		return r.done(err)
	}
	return r.done(r.run(ctx))
}

func (r *evalRun) run(ctx context.Context) error {
	if err := r.graph.Load(ctx); err != nil {
		return errors.Annotate(err, "failed to load the file graph").Err()
	}

	r.ev.Eval.Strategy = r.selectTests
	res, err := r.ev.Run(ctx)
	if err != nil {
		return err
	}
	res.Print(os.Stdout, 0.9)
	return nil
}
