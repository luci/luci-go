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
	"context"
	"fmt"
	"os"

	"github.com/golang/protobuf/jsonpb"
	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"

	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// queryRun is a base subcommandRun for subcommands ls and derive.
type queryRun struct {
	baseCommandRun
	limit int

	// TODO(crbug.com/1021849): add flag -test-path
	// TODO(crbug.com/1021849): add flag -include-expected
	// TODO(crbug.com/1021849): add flag -artifact-dir
	// TODO(crbug.com/1021849): add flag -artifact-name
}

func (r *queryRun) registerFlags(p Params) {
	r.RegisterGlobalFlags(p)
	r.RegisterJSONFlag()
	r.Flags.IntVar(&r.limit, "n", 0, text.Doc(`
		Print up to n results of each result type. If 0, then unlimited.
	`))
}

func (r *queryRun) validate() error {
	if r.limit < 0 {
		return errors.Reason("-n must be non-negative").Err()
	}

	// TODO(crbug.com/1021849): improve validation.
	return nil
}

// queryAndPrint queries results and prints them.
func (r *queryRun) queryAndPrint(ctx context.Context, inv *pb.InvocationPredicate) error {
	// TODO(crbug.com/1021849): implement paging.
	res, err := r.resultdb.QueryTestResults(ctx, &pb.QueryTestResultsRequest{
		Predicate: &pb.TestResultPredicate{
			Invocation: inv,
		},
		PageSize: int32(r.limit),
	})
	if err != nil {
		return err
	}

	if !r.json {
		// TODO(crbug.com/1021849): implement human-oriented output.
		return errors.Reason("unimplemented").Err()
	}

	// TODO(crbug.com/1021849): query test exonerations.

	m := jsonpb.Marshaler{
		Indent: "  ",
	}
	for _, res := range res.TestResults {
		m.Marshal(os.Stdout, res)
		fmt.Println()
	}
	return nil
}
