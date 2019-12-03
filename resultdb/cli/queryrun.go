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

type bundle struct {
	Err              error
	InvocationID     string
	TestResults      []*pb.TestResult
	TestExonerations []*pb.TestExoneration
}

// JSONish converts b to a JSONish-marshalable object.
func (b *bundle) JSONish() interface{} {
	testResults := make([]json.RawMessage, len(b.TestResults))
	for i, tr := range b.TestResults {
		testResults[i] = msgToJSON(tr)
	}

	testExonerations := make([]json.RawMessage, len(b.TestExonerations))
	for i, te := range b.TestExonerations {
		testExonerations[i] = msgToJSON(te)
	}

	obj := map[string]interface{}{
		"testResults":      testResults,
		"testExonerations": testExonerations,
	}
	if b.InvocationID != "" {
		obj["invocationId"] = b.InvocationID
	}
	return obj
}

// queryAndPrint queries results and prints them.
func (r *queryRun) queryAndPrint(ctx context.Context, merge bool, invIDs []string) error {
	bundleC := make(chan *bundle, 1)
	if merge {
		bundleC <- r.fetch(ctx, invIDs)
		close(bundleC)
	} else {
		go func() {
			defer close(bundleC)
			r.fetchSeparately(ctx, invIDs, bundleC)
		}()
	}

	if r.json {
		r.printJSON(bundleC)
		return nil
	}

	return errors.Reason("unimplemented").Err()
}

// fetchSeparately fetches results of each invocation separately.
// Returned bundles have initialized InvocationID.
func (r *queryRun) fetchSeparately(ctx context.Context, invIDs []string, bundleC chan<- *bundle) {
	ordered := make([]chan *bundle, len(invIDs))
	for i, id := range invIDs {
		i := i
		id := id
		c := make(chan *bundle, 1)
		ordered[i] = c
		go func() {
			c <- r.fetch(ctx, []string{id})
		}()
	}

	for _, c := range ordered {
		b := <-c
		bundleC <- b
	}
}

// fetch fetches test results and exonerations from the specified invocations.
// Does not set bundle.InvocationID.
func (r *queryRun) fetch(ctx context.Context, invIDs []string) *bundle {
	b := &bundle{}

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
		b.TestResults = res.TestResults
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
		b.TestExonerations = res.TestExonerations
		return nil
	})

	b.Err = eg.Wait()
	return b
}

func (r *queryRun) printJSON(bundleC <-chan *bundle) {
	enc := json.NewEncoder(os.Stdout)
	for b := range bundleC {
		enc.Encode(b.JSONish())
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
