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
	"fmt"
	"os"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"
	luciflag "go.chromium.org/luci/common/flag"

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

func cmdLs(p Params) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `ls [flags]`,
		ShortDesc: "query results",
		CommandRun: func() subcommands.CommandRun {
			r := &lsRun{
				tags: strpair.Map{},
			}
			r.queryRun.registerFlags(p)
			r.Flags.Var(luciflag.StringSlice(&r.invIDs), "inv", text.Doc(`
				Retrieve results from the invocation with this ID.

				May be specified multiple times and compatible with -tag:
				tags and invocation ids are connected with logical OR.
			`))

			r.Flags.Var(luciflag.StringPairs(r.tags), "tag", text.Doc(`
				Retrieve results from invocations having this tag.

				May be specified multiple times and compatible with -inv:
				tags and invocation ids are connected with logical OR.
			`))

			// TODO(crbug.com/1021849): add flag -cl
			// TODO(crbug.com/1021849): add flag -var
			// TODO(crbug.com/1021849): add flag -watch

			return r
		},
	}
}

type lsRun struct {
	queryRun
	invIDs []string
	tags   strpair.Map
}

func (r *lsRun) Validate() error {
	if len(r.invIDs) == 0 && len(r.tags) == 0 {
		return errors.Reason("-inv or -tag are required").Err()
	}

	return r.queryRun.validate()
}

func (r *lsRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)

	if err := r.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	if err := r.initClients(ctx); err != nil {
		return r.done(err)
	}

	return r.done(r.queryAndPrint(ctx, r.InvocationPredicate()))
}

func (r *lsRun) InvocationPredicate() *pb.InvocationPredicate {
	ret := &pb.InvocationPredicate{
		Names: make([]string, len(r.invIDs)),
		Tags:  pbutil.FromStrpairMap(r.tags),
	}
	for i, id := range r.invIDs {
		ret.Names[i] = pbutil.InvocationName(id)
	}
	return ret
}
