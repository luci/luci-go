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

package ledcli

import (
	"context"
	"net/http"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"

	"go.chromium.org/luci/led/job"
	"go.chromium.org/luci/led/ledcmd"
)

func editIsolated(opts cmdBaseOptions) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "edit-isolated [transform_program args...]",
		ShortDesc: "Allows arbitrary local edits to the isolated input.",
		LongDesc: `Downloads the task isolated (if any) into a temporary folder,
and then waits for your edits.

If you don't specify "transform_program", this will prompt with the location of
the temporary folder, and will wait for you to hit <enter>. You may manually
edit the contents of the folder however you like, and on <enter> the contents
will be isolated and deleted, and the new isolated will be attached to the task.

If "transform_program" and any arguments are specified, it will be run like:

   cd /path/to/isolated/dir && transform_program args...

And there will be no interactive prompt. All stdout/stderr from
transform_program will be redirected to stderr.
`,

		CommandRun: func() subcommands.CommandRun {
			ret := &cmdEditIsolated{}
			ret.initFlags(opts)
			return ret
		},
	}
}

type cmdEditIsolated struct {
	cmdBase

	transformProgram []string
}

func (c *cmdEditIsolated) jobInput() bool                  { return true }
func (c *cmdEditIsolated) positionalRange() (min, max int) { return 0, 99999 }

func (c *cmdEditIsolated) validateFlags(ctx context.Context, positionals []string, env subcommands.Env) error {
	c.transformProgram = positionals
	return nil
}

func (c *cmdEditIsolated) execute(ctx context.Context, _ *http.Client, authOpts auth.Options, inJob *job.Definition) (output any, err error) {
	var xform ledcmd.IsolatedTransformer
	if len(c.transformProgram) == 0 {
		xform = ledcmd.PromptIsolatedTransformer()
	} else {
		xform = ledcmd.ProgramIsolatedTransformer(c.transformProgram...)
	}
	return inJob, ledcmd.EditIsolated(ctx, authOpts, inJob, xform)
}

func (c *cmdEditIsolated) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	return c.doContextExecute(a, c, args, env)
}
