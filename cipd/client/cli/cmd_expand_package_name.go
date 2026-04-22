// Copyright 2026 The LUCI Authors.
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
	"fmt"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/cipd/client/cipd/template"
)

////////////////////////////////////////////////////////////////////////////////
// 'expand-package-name' subcommand.

func cmdExpandPackageName(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "expand-package-name",
		ShortDesc: "replaces any placeholder variables in the given package name",
		LongDesc: "Replaces any placeholder variables in the given package " +
			"name.\n If supplying a name using the feature ${var=possible,values} " +
			"an empty string will be returned if the expansion does not match the " +
			"current variable state.",
		Advanced: true,
		CommandRun: func() subcommands.CommandRun {
			c := &expandPackageNameRun{}
			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir, withoutMaxThreads)
			return c
		},
	}
}

type expandPackageNameRun struct {
	cipdSubcommand
	clientOptions
}

func (c *expandPackageNameRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 0, 1) {
		return 1
	}

	if len(args) == 1 {
		path, err := template.DefaultExpander().Expand(args[0])
		if err != nil {
			if !errors.Is(err, template.ErrSkipTemplate) {
				return c.done(nil, err)
			}
			// return an empty string if the variable expansion does not
			// apply to current system.
			path = ""
		}

		fmt.Println(path)

		return c.done(path, nil)
	}

	return c.done("", makeCLIError("one package name must be supplied: %v", args))
}
