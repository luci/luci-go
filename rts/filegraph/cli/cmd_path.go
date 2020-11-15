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
	"fmt"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"
)

var cmdPath = &subcommands.Command{
	UsageLine: `path [flags] SOURCE_FILE TARGET_FILE`,
	ShortDesc: "print the shortest path from SOURCE_FILE to TARGET_FILE",
	LongDesc: text.Doc(`
		Print the shortest path from SOURCE_FILE to TARGET_FILE

		Each output line has format "<distance> (+<delta>) <filename>",
		where the filename is forward-slash-separated and has "//" prefix.
		Output example:
			0.00 (+0.00) //source_file.cc
			1.00 (+1.00) //intermediate_file.cc
			3.00 (+2.00) //target_file.cc

		Both files must be in the same git repository.
	`),
	CommandRun: func() subcommands.CommandRun {
		r := &pathRun{}
		r.gitGraph.RegisterFlags(&r.Flags)
		return r
	},
}

type pathRun struct {
	baseCommandRun
	gitGraph
}

func (r *pathRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)

	if len(args) != 2 {
		return r.done(errors.Reason("usage: filegraph path SOURCE_FILE TARGET_FILE").Err())
	}

	nodes, err := r.loadSyncedNodes(ctx, args[0], args[1])
	if err != nil {
		return r.done(err)
	}

	r.q.Sources = nodes[:1]
	shorest := r.q.ShortestPath(nodes[1])
	if shorest == nil {
		return r.done(errors.New("not reachable"))
	}

	prevDist := 0.0
	for _, sp := range shorest.Path() {
		fmt.Printf("%.2f (+%.2f) %s\n", sp.Distance, sp.Distance-prevDist, sp.Node.Name())
		prevDist = sp.Distance
	}
	return 0
}
