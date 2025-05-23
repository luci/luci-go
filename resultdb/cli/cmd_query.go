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
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/maruel/subcommands"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/data/text/indented"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func cmdQuery(p Params) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `query [flags] [INVOCATION_ID]...`,
		ShortDesc: "query results",
		LongDesc: text.Doc(`
			Query results.

			Most users will be interested only in results of test variants that had
			unexpected results. This can be achieved by passing -u flag.
			This significantly reduces output size and latency.

			If no invocation ids are specified on the command line, read them from
			stdin separated by newline. Example:
			  bb chromium/ci/linux-rel -status failure -inv -10 | rdb query
		`),
		CommandRun: func() subcommands.CommandRun {
			r := &queryRun{}
			r.RegisterGlobalFlags(p)
			r.RegisterJSONFlag(text.Doc(`
				Print results in JSON format separated by newline.
				One result takes exactly one line. Result object properties are invocationId
				and one of
					testResult: luci.resultdb.v1.TestResult message.
					testExoneration: luci.resultdb.v1.TestExoneration message.
					invocation: luci.resultdb.v1.Invocation message.
			`))

			r.Flags.IntVar(&r.limit, "n", 0, text.Doc(`
				Print up to n results. If 0, then unlimited.
				Invocations do not count as results.
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

			r.Flags.StringVar(&r.trFields, "tr-fields", "", text.Doc(`
				Test result fields to include in the response. Fields should be passed
				as a comma separated string to match the JSON encoding schema of FieldMask.
				Test result names will always be included even if "name" is not a part
				of the fields.
			`))
			return r
		},
	}
}

type queryRun struct {
	baseCommandRun

	limit      int
	unexpected bool
	testID     string
	merge      bool
	trFields   string
	invIDs     []string

	// TODO(crbug.com/1021849): add flag -artifact-dir
	// TODO(crbug.com/1021849): add flag -artifact-name
}

func (r *queryRun) parseArgs(args []string) error {
	r.invIDs = args
	if len(r.invIDs) == 0 {
		var err error
		if r.invIDs, err = readStdin(); err != nil {
			return err
		}
	}

	for _, id := range r.invIDs {
		if err := pbutil.ValidateInvocationID(id); err != nil {
			return errors.Annotate(err, "invocation id %q", id).Err()
		}
	}

	if r.limit < 0 {
		return errors.Reason("-n must be non-negative").Err()
	}

	// TODO(crbug.com/1021849): improve validation.
	return nil
}

func (r *queryRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)

	if err := r.parseArgs(args); err != nil {
		return r.done(err)
	}

	if err := r.initClients(ctx, auth.SilentLogin); err != nil {
		return r.done(err)
	}

	return r.done(r.queryAndPrint(ctx, r.invIDs))
}

// readStdin reads all lines from os.Stdin.
func readStdin() ([]string, error) {
	// This context is used only to cancel the goroutine below.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-time.After(time.Second):
			fmt.Fprintln(os.Stderr, "expecting invocation ids on the command line or stdin...")
		case <-ctx.Done():
		}
	}()

	var ret []string
	stdin := bufio.NewReader(os.Stdin)
	for {
		line, err := stdin.ReadString('\n')
		if err == io.EOF {
			return ret, nil
		}
		if err != nil {
			return nil, err
		}
		ret = append(ret, strings.TrimSuffix(line, "\n"))
		cancel() // do not print the warning since we got something.
	}
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
		eg.Go(func() error {
			return r.fetchInvocation(ctx, id, resultC)
		})
	}

	trMask := &fieldmaskpb.FieldMask{}
	if r.trFields != "" {
		if err := protojson.Unmarshal([]byte(fmt.Sprintf(`"%s"`, r.trFields)), trMask); err != nil {
			return errors.Annotate(err, "tr-fields").Err()
		}
	}

	// Fetch items into resultC.
	if r.merge {
		eg.Go(func() error {
			return r.fetchItems(ctx, invIDs, trMask, resultItem{}, resultC)
		})
	} else {
		for _, id := range invIDs {
			tmpl := resultItem{invocationID: id}
			eg.Go(func() error {
				return r.fetchItems(ctx, []string{id}, trMask, tmpl, resultC)
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

	r.printProto(resultC, r.json)
	return <-errC
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
func (r *queryRun) fetchItems(ctx context.Context, invIDs []string, trMask *fieldmaskpb.FieldMask, resultItemTemplate resultItem, dest chan<- resultItem) error {
	invNames := make([]string, len(invIDs))
	for i, id := range invIDs {
		invNames[i] = pbutil.InvocationName(id)
	}

	// Prepare a test result request.
	trReq := &pb.QueryTestResultsRequest{
		Invocations: invNames,
		Predicate:   &pb.TestResultPredicate{TestIdRegexp: r.testID},
		PageSize:    int32(r.limit),
		ReadMask:    trMask,
	}
	if r.unexpected {
		trReq.Predicate.Expectancy = pb.TestResultPredicate_VARIANTS_WITH_UNEXPECTED_RESULTS
	}

	// Prepare a test exoneration request.
	teReq := &pb.QueryTestExonerationsRequest{
		Invocations: invNames,
		Predicate: &pb.TestExonerationPredicate{
			TestIdRegexp: r.testID,
		},
		PageSize: int32(r.limit),
	}

	// Query for results.
	msgC := make(chan proto.Message)
	errC := make(chan error, 1)
	queryCtx, cancelQuery := context.WithCancel(ctx)
	defer cancelQuery()
	go func() {
		defer close(msgC)
		errC <- pbutil.Query(queryCtx, msgC, r.resultdb, trReq, teReq)
	}()

	// Send findings to the destination channel.
	count := 0
	reachedLimit := false
	for m := range msgC {
		if r.limit > 0 && count > r.limit {
			reachedLimit = true
			cancelQuery()
			break
		}
		count++

		item := resultItemTemplate
		item.result = m
		select {
		case dest <- item:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// Return the query error.
	err := <-errC
	if reachedLimit && err == context.Canceled {
		err = nil
	}
	return err
}

// printProto prints results in JSON or TextProto format to stdout.
// Each result takes exactly one line and is followed by newline.
//
// The printed JSON supports streaming, and is easy to parse by languages (Python)
// that cannot parse an arbitrary sequence of JSON values.
func (r *queryRun) printProto(resultC <-chan resultItem, printJSON bool) {
	enc := json.NewEncoder(os.Stdout)
	ind := &indented.Writer{
		Writer:    os.Stdout,
		UseSpaces: true,
	}
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

		if printJSON {
			obj := map[string]any{
				key: json.RawMessage(msgToJSON(res.result)),
			}
			if !r.merge {
				obj["invocationId"] = res.invocationID
			}
			enc.Encode(obj) // prints \n in the end
		} else {
			fmt.Fprintf(ind, "%s:\n", key)
			ind.Level++
			proto.MarshalText(ind, res.result)
			ind.Level--
			fmt.Fprintln(ind)
		}
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
