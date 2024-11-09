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

package isolateimpl

import (
	"fmt"
	"os"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/client/isolate"
	"go.chromium.org/luci/common/errors"
)

// CmdRemap returns an object for the `remap` subcommand.
func CmdRemap() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "remap <options>",

		ShortDesc: "Creates a directory with all the dependencies mapped into it.",
		LongDesc: `Creates a directory with all the dependencies mapped into it.

Useful to test manually why a test is failing. The target executable is not run.`,
		CommandRun: func() subcommands.CommandRun {
			r := remapRun{}
			r.baseCommandRun.Init()
			r.isolateFlags.Init(&r.Flags)
			r.Flags.StringVar(&r.outdir, "outdir", "", "Directory used to recreate the tree.")
			return &r
		},
	}
}

type remapRun struct {
	baseCommandRun
	isolateFlags

	outdir string
}

func (r *remapRun) Parse(a subcommands.Application, args []string) error {
	if err := r.baseCommandRun.Parse(); err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := r.isolateFlags.Parse(cwd); err != nil {
		return err
	}

	if len(args) != 0 {
		return errors.Reason("position arguments not expected").Err()
	}

	if r.outdir == "" {
		return errors.Reason("-outdir is not specified").Err()
	}

	return nil
}

func (r *remapRun) Run(a subcommands.Application, args []string, _ subcommands.Env) int {
	if err := r.Parse(a, args); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	if err := r.main(a, args); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	return 0
}

func (r *remapRun) main(a subcommands.Application, args []string) error {
	deps, rootDir, err := isolate.ProcessIsolate(&r.ArchiveOptions)
	if err != nil {
		return errors.Annotate(err, "failed to process isolate").Err()
	}

	return recreateTree(r.outdir, rootDir, deps)
}
