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

	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// queryRun is a base subcommandRun for subcommands ls and derive.
type queryRun struct {
	baseCommandRun
	limit      int
	unexpected bool
	testID     string
	merge      bool

	// TODO(crbug.com/1021849): add flag -artifact-dir
	// TODO(crbug.com/1021849): add flag -artifact-name
}

func (r *queryRun) registerFlags(p Params) {
	r.RegisterGlobalFlags(p)
	r.RegisterJSONFlag(text.Doc(`
		Print results in JSON format separated by newline.
		One result takes exactly one line. Result object properties are invocationId
		and one of
		  testResult: luci.resultdb.rpc.v1.TestResult message.
			testExoneration: luci.resultdb.rpc.v1.TestExoneration message.
			invocation: luci.resultdb.rpc.v1.Invocation message.
	`))

	r.Flags.IntVar(&r.limit, "n", 0, text.Doc(`
		Print up to n results of each result type. If 0, then unlimited.
	`))

	r.Flags.BoolVar(&r.unexpected, "u", false, text.Doc(`
		Print only test results of test variants that have unexpected results.
		For example, if a test variant expected PASS and had results FAIL, FAIL,
		PASS, then print all of them.
		This signficantly reduces output size and latency.
	`))

	r.Flags.StringVar(&r.testID, "test", "", text.Doc(`
		A regular expression for test id. Implicitly wrapped with ^ and $.

		Example: ninja://chrome/test:browser_tests/.+
	`))

	r.Flags.BoolVar(&r.merge, "merge", false, text.Doc(`
		Merge results of the invocations, as if they were included into one
		invocation.
		Useful when the invocations are a part of one computation, e.g. shards
		of a test.
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
	invocationID string
	result       proto.Message
}

// queryAndPrint queries results and prints them.
func (r *queryRun) queryAndPrint(ctx context.Context, invIDs []string) error {
	eg, ctx := errgroup.WithContext(ctx)
	resultC := make(chan resultItem)

	for _, id := range invIDs {
		id := id
		eg.Go(func() error {
			return r.fetchInvocation(ctx, id, resultC)
		})
	}

	// Fetch items into resultC.
	if r.merge {
		eg.Go(func() error {
			return r.fetchItems(ctx, invIDs, resultItem{}, resultC)
		})
	} else {
		for _, id := range invIDs {
			tmpl := resultItem{invocationID: id}
			eg.Go(func() error {
				return r.fetchItems(ctx, []string{id}, tmpl, resultC)
			})
		}
	}

	// Wait for fetchers to finish and close resultC.
	errC := make(chan error)
	go func() {
		err := eg.Wait()
		close(resultC)
		errC <- err
	}()

	if r.json {
		r.printJSON(resultC)
		return <-errC
	}

	return errors.Reason("-json required; human output is not implemented yet").Err()
}

// fetchInvocation fetches an invocation.
func (r *queryRun) fetchInvocation(ctx context.Context, invID string, dest chan<- resultItem) error {
	res, err := r.resultdb.GetInvocation(ctx, &pb.GetInvocationRequest{Name: pbutil.InvocationName(invID)})
	if err != nil {
		return err
	}
	dest <- resultItem{invocationID: invID, result: res}
	return nil
}

// fetchItems fetches test results and exonerations from the specified invocations.
func (r *queryRun) fetchItems(ctx context.Context, invIDs []string, resultItemTemplate resultItem, dest chan<- resultItem) error {
	invNames := make([]string, len(invIDs))
	for i, id := range invIDs {
		invNames[i] = pbutil.InvocationName(id)
	}

	eg, ctx := errgroup.WithContext(ctx)

	// Fetch test results.
	eg.Go(func() error {
		req := &pb.QueryTestResultsRequest{
			Invocations: invNames,
			Predicate:   &pb.TestResultPredicate{TestIdRegexp: r.testID},
			PageSize:    int32(r.limit),
		}
		if r.unexpected {
			req.Predicate.Expectancy = pb.TestResultPredicate_VARIANTS_WITH_UNEXPECTED_RESULTS
		}
		// TODO(crbug.com/1021849): implement paging.
		res, err := r.resultdb.QueryTestResults(ctx, req)
		if err != nil {
			return err
		}
		for _, tr := range res.TestResults {
			item := resultItemTemplate
			item.result = tr
			select {
			case dest <- item:
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
				TestIdRegexp: r.testID,
			},
			PageSize: int32(r.limit),
		}
		// TODO(crbug.com/1021849): implement paging.
		res, err := r.resultdb.QueryTestExonerations(ctx, req)
		if err != nil {
			return err
		}
		for _, te := range res.TestExonerations {
			item := resultItemTemplate
			item.result = te
			select {
			case dest <- item:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	})

	return eg.Wait()
}

// printJSON prints results in JSON format to stdout.
// Each result takes exactly one line and is followed by newline.
// This format supports streaming, and is easy to parse by languages (Python)
// that cannot parse an arbitrary sequence of JSON values.
func (r *queryRun) printJSON(resultC <-chan resultItem) {
	enc := json.NewEncoder(os.Stdout)
	for res := range resultC {
		var key string
		switch res.result.(type) {
		case *pb.TestResult:
			key = "testResult"
		case *pb.TestExoneration:
			key = "testExoneration"
		case *pb.Invocation:
			key = "invocation"
		default:
			panic(fmt.Sprintf("unexpected result type %T", res.result))
		}

		obj := map[string]interface{}{
			key: json.RawMessage(msgToJSON(res.result)),
		}
		if !r.merge {
			obj["invocationId"] = res.invocationID
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
