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
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/rts/filegraph"
)

var cmdPath = &subcommands.Command{
	UsageLine: `path`,
	ShortDesc: "path",
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
		return r.done(errors.Reason("usage: filegraph path <from> <to>").Err())
	}

	nodes, err := r.loadSyncedNodes(ctx, args[0], args[1])
	if err != nil {
		return r.done(err)
	}

	q := filegraph.Query{Sources: nodes[:1]}
	shorest := q.ShortestPath(nodes[1])
	if shorest == nil {
		return r.done(errors.Reason("not reachable").Err())
	}

	d := 0.0
	for _, sp := range shorest.Path() {
		fmt.Printf("%.2f (+%.2f) %s\n", sp.Distance, sp.Distance-d, sp.Node.Name())
		d = sp.Distance
	}
	return 0
}
