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
	"flag"
	"fmt"
	"os"

	"github.com/golang/protobuf/jsonpb"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/resultdb/pbutil"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"
	luciflag "go.chromium.org/luci/common/flag"

	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

type queryFlags struct {
	invIDs []string
	tags   strpair.Map
	limit  int
}

func (f *queryFlags) Register(fs *flag.FlagSet) {
	fs.Var(luciflag.StringSlice(&f.invIDs), "inv", text.Doc(`
		Retrieve results from this invocation.

		May be specified multiple times and compatible with -tag:
		tags and invocation ids are connected with logical OR.
	`))

	fs.Var(luciflag.StringPairs(f.tags), "tag", text.Doc(`
		Retrieve results from invocations having this tag.

		May be specified multiple times and compatible with -inv:
		tags and invocation ids are connected with logical OR.
	`))

	fs.IntVar(&f.limit, "n", 0, text.Doc(`
		Print up to n results of each result type. If 0, then unlimited.
	`))
}

func (f *queryFlags) Validate() error {
	if len(f.invIDs) == 0 && len(f.tags) == 0 {
		return errors.Reason("-inv or -tag are required").Err()
	}
	return nil
}

func (f *queryFlags) InvocationPredicate() *pb.InvocationPredicate {
	ret := &pb.InvocationPredicate{
		Names: make([]string, len(f.invIDs)),
		Tags:  pbutil.FromStrpairMap(f.tags),
	}
	for i, id := range f.invIDs {
		ret.Names[i] = pbutil.InvocationName(id)
	}
	return ret
}

func (f *queryFlags) TestResultRequest() *pb.QueryTestResultsRequest {
	return &pb.QueryTestResultsRequest{
		Predicate: &pb.TestResultPredicate{
			Invocation: f.InvocationPredicate(),
		},
		PageSize: int32(f.limit),
	}
}

func cmdLs(p Params) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `ls [flags]`,
		ShortDesc: "query results",
		CommandRun: func() subcommands.CommandRun {
			r := &lsRun{}
			r.tags = strpair.Map{}
			r.RegisterDefaultFlags(p)
			r.RegisterJSONFlag()
			r.queryFlags.Register(&r.Flags)
			return r
		},
	}
}

type lsRun struct {
	baseCommandRun
	queryFlags
}

func (r *lsRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)

	if err := r.queryFlags.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	if err := r.initClients(ctx); err != nil {
		return r.done(ctx, err)
	}

	if r.limit < 0 {
		return r.done(ctx, fmt.Errorf("-n value must be non-negative"))
	}

	ctx = metadata.AppendToOutgoingContext(ctx, "host", "nodir.resultdb.apis.chromium.org")

	// TODO(nodir): implement paging.
	res, err := r.resultdb.QueryTestResults(ctx, r.TestResultRequest())
	if err != nil {
		return r.done(ctx, err)
	}

	if !r.json {
		// TODO(nodir): implement.
		return r.done(ctx, errors.Reason("unimplemented").Err())
	}

	m := jsonpb.Marshaler{
		Indent: "  ",
	}
	for _, res := range res.TestResults {
		m.Marshal(os.Stdout, res)
		fmt.Println()
	}
	return 0
}
