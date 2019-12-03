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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// queryRun is a base subcommandRun for subcommands ls and derive.
type queryRun struct {
	baseCommandRun
	limit              int
	ignoreExpectations bool
	testPath           string

	// TODO(crbug.com/1021849): add flag -artifact-dir
	// TODO(crbug.com/1021849): add flag -artifact-name
}

func (r *queryRun) registerFlags(p Params) {
	r.RegisterGlobalFlags(p)
	r.RegisterJSONFlag()

	r.Flags.IntVar(&r.limit, "n", 0, help(`
		Print up to n results of each result type. If 0, then unlimited.
	`))

	r.Flags.BoolVar(&r.ignoreExpectations, "ignore-expectations", false, help(`
		Do not filter based on whether the result was expected.
		Note that this may significantly increase output size and latency.

		If false (default), print only results of test variants that have unexpected
		results.
		For example, if a test variant expected PASS and had results FAIL, FAIL,
		PASS, then print all of them.
	`))

	r.Flags.StringVar(&r.testPath, "test-path", "", help(`
		A regular expression for test path. Implicitly wrapped with ^ and $.

		Example: gn://chrome/test:browser_tests/.+
	`))
}

func (r *queryRun) validate() error {
	if r.limit < 0 {
		return errors.Reason("-n must be non-negative").Err()
	}

	// TODO(crbug.com/1021849): improve validation.
	return nil
}

type resultItem struct {
	invocationIDs []string
	result        proto.Message
}

// queryAndPrint queries results and prints them.
func (r *queryRun) queryAndPrint(ctx context.Context, merge bool, invIDs []string) error {
	var invIDGroups [][]string
	if merge {
		invIDGroups = [][]string{invIDs}
	} else {
		invIDGroups = make([][]string, len(invIDs))
		for i, id := range invIDs {
			invIDGroups[i] = []string{id}
		}
	}

	resultC := make(chan resultItem)
	eg, ctx := errgroup.WithContext(ctx)
	for _, ids := range invIDGroups {
		ids := ids
		eg.Go(func() error {
			return r.fetch(ctx, ids, resultC)
		})
	}

	errC := make(chan error)
	go func() {
		err := eg.Wait()
		close(resultC)
		errC <- err
	}()

	if r.json {
		r.printJSON(resultC, merge)
		return nil
	}

	return errors.Reason("unimplemented").Err()
}

// fetch fetches test results and exonerations from the specified invocations.
// Does not set bundle.InvocationID.
func (r *queryRun) fetch(ctx context.Context, invIDs []string, dest chan<- resultItem) error {
	invNames := make([]string, len(invIDs))
	for i, id := range invIDs {
		invNames[i] = pbutil.InvocationName(id)
	}

	eg, ctx := errgroup.WithContext(ctx)

	// Fetch test results.
	eg.Go(func() error {
		req := &pb.QueryTestResultsRequest{
			Invocations: invNames,
			Predicate: &pb.TestResultPredicate{
				TestPathRegexp: r.testPath,
				Expectancy:     pb.TestResultPredicate_VARIANTS_WITH_UNEXPECTED_RESULTS,
			},
			PageSize: int32(r.limit),
		}
		if r.ignoreExpectations {
			req.Predicate.Expectancy = pb.TestResultPredicate_ALL
		}
		// TODO(crbug.com/1021849): implement paging.
		res, err := r.resultdb.QueryTestResults(ctx, req)
		if err != nil {
			return err
		}
		for _, tr := range res.TestResults {
			select {
			case dest <- resultItem{invocationIDs: invIDs, result: tr}:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	})

	// Fetch test exonerations.
	eg.Go(func() error {
		req := &pb.QueryTestExonerationsRequest{
			Invocations: invNames,
			Predicate: &pb.TestExonerationPredicate{
				TestPathRegexp: r.testPath,
			},
			PageSize: int32(r.limit),
		}
		// TODO(crbug.com/1021849): implement paging.
		res, err := r.resultdb.QueryTestExonerations(ctx, req)
		if err != nil {
			return err
		}
		for _, te := range res.TestExonerations {
			select {
			case dest <- resultItem{invocationIDs: invIDs, result: te}:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	})

	return eg.Wait()
}

func (r *queryRun) printJSON(resultC <-chan resultItem, merge bool) {
	enc := json.NewEncoder(os.Stdout)
	for res := range resultC {
		var key string
		switch res.result.(type) {
		case *pb.TestResult:
			key = "testResult"
		case *pb.TestExoneration:
			key = "testExoneration"
		default:
			panic(fmt.Sprintf("unexpected result type %T", res.result))
		}

		obj := map[string]interface{}{
			key: json.RawMessage(msgToJSON(res.result)),
		}
		if !merge {
			if len(res.invocationIDs) != 1 {
				panic("impossible")
			}
			obj["invocationId"] = res.invocationIDs[0]
		}
		enc.Encode(obj) // prints \n in the end
	}
}

func msgToJSON(msg proto.Message) []byte {
	buf := &bytes.Buffer{}
	m := jsonpb.Marshaler{}
	if err := m.Marshal(buf, msg); err != nil {
		panic(fmt.Sprintf("failed to marshal protobuf message %q in memory", msg))
	}
	return buf.Bytes()
}
