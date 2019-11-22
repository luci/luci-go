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
	"strings"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"
	luciflag "go.chromium.org/luci/common/flag"

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// queryRun is a base subcommandRun for subcommands ls and derive.
type queryRun struct {
	baseCommandRun
	limit              int
	ignoreExpectations bool
	testPaths          []string

	// TODO(crbug.com/1021849): add flag -artifact-dir
	// TODO(crbug.com/1021849): add flag -artifact-name
}

func (r *queryRun) registerFlags(p Params) {
	r.RegisterGlobalFlags(p)
	r.RegisterJSONFlag()

	r.Flags.IntVar(&r.limit, "n", 0, text.Doc(`
		Print up to n results of each result type. If 0, then unlimited.
	`))

	r.Flags.BoolVar(&r.ignoreExpectations, "ignore-expectations", false, text.Doc(`
		Do not filter based on whether the result was expected.
		Note that this may significantly increase output size and latency.

		If false (default), print only results of test variants that have unexpected
		results.
		For example, if a test variant expected PASS and had results FAIL, FAIL,
		PASS, then print all of them.
	`))

	r.Flags.Var(luciflag.StringSlice(&r.testPaths), "test-path", text.Doc(`
		Filter by test path. If ends with "*", then treated as a prefix, e.g.
		gn://chrome/test:browser_tests/*

		May be specified multiple times. Test paths are connected with logical OR.
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
	req := &pb.QueryTestResultsRequest{
		Predicate: &pb.TestResultPredicate{
			Invocation: inv,
			TestPath:   &pb.TestPathPredicate{},
			Expectancy: pb.TestResultPredicate_VARIANTS_WITH_UNEXPECTED_RESULTS,
		},
		PageSize: int32(r.limit),
	}

	if r.ignoreExpectations {
		req.Predicate.Expectancy = pb.TestResultPredicate_ALL
	}
	for _, p := range r.testPaths {
		dest := &req.Predicate.TestPath.Paths
		if strings.HasSuffix(p, "*") {
			p = strings.TrimSuffix(p, "*")
			dest = &req.Predicate.TestPath.PathPrefixes
		}
		*dest = append(*dest, p)
	}

	itemC := make(chan proto.Message)
	errC := make(chan error)
	queryCtx, stopQuering := context.WithCancel(ctx)
	defer stopQuering()
	go func() {
		err := pbutil.QueryResults(queryCtx, itemC, r.resultdb, req)
		close(itemC)
		errC <- err
	}()

	// TODO(crbug.com/1021849): query test exonerations.

	return r.printItems(itemC)
}

// printItems test results and exonerations in itemC to stdout.
func (r *queryRun) printItems(itemC chan proto.Message) error {
	if !r.json {
		// TODO(crbug.com/1021849): implement human-oriented output.
		return errors.Reason("unimplemented").Err()
	}

	m := jsonpb.Marshaler{}
	buf := &bytes.Buffer{}
	for item := range itemC {
		var key string
		switch item.(type) {
		case *pb.TestResult:
			key = "testResult"
		case *pb.TestExoneration:
			key = "testExoneration"
		default:
			panic("impossible")
		}

		buf.Reset()
		if err := m.Marshal(buf, item); err != nil {
			return err
		}
		envelope := map[string]interface{}{key: json.RawMessage(buf.Bytes())}
		envelopeJSON, err := json.MarshalIndent(envelope, "", "  ")
		if err != nil {
			return err
		}
		fmt.Printf("%s\n", envelopeJSON)
	}
	return nil
}
