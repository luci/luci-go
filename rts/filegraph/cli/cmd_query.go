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

package cli

import (
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/data/text"
)

var cmdQuery = &subcommands.Command{
	UsageLine: `query [flags] [PATH [PATH...]]`,
	ShortDesc: "retrieve files in the descending order of distance",
	LongDesc: text.Doc(`
		Retrieve files in the descending order of distance from the given one.

		Each PATH must be a file in a git repository.
		All files must be in the same repository.

		Prints other files in the order from the most relevant to least relevant.
		Each line has format "<distance> <filename>",
		where the filename is relative to the repo root, e.g. "foo/bar.cpp".
	`),
	CommandRun: func() subcommands.CommandRun {
		r := &queryRun{}
		return r
	},
}

type queryRun struct {
	baseCommandRun
}

func (r *queryRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	panic("not implemented")
}
