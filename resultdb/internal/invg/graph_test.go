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

package invg

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"go.chromium.org/luci/common/clock/testclock"

	"go.chromium.org/luci/resultdb/internal/span"

	"go.chromium.org/luci/resultdb/internal/testutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestFetchGraph(t *testing.T) {
	Convey(`TestInclude`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		t0 := testclock.TestRecentTimeUTC
		t1 := t0.Add(time.Hour)
		t2 := t1.Add(time.Hour)
		t3 := t2.Add(time.Hour)

		fetch := func(roots ...span.InvocationID) (*Graph, error) {
			txn := span.Client(ctx).ReadOnlyTransaction()
			defer txn.Close()
			return FetchGraph(ctx, txn, roots...)
		}

		mustFetch := func(roots ...span.InvocationID) *Graph {
			g, err := fetch(roots...)
			So(err, ShouldBeNil)
			return g
		}

		assertGraph := func(g *Graph, expected string) {
			buf := &bytes.Buffer{}
			g.Dump(buf)
			actual := strings.TrimSpace(buf.String())

			lines := strings.Split(expected, "\n")
			for i := range lines {
				lines[i] = strings.TrimSpace(lines[i])
			}
			expected = strings.TrimSpace(strings.Join(lines, "\n"))
			So(actual, ShouldEqual, expected)
		}

		Convey(`fetch nothing`, func() {
			assertGraph(mustFetch(), ``)
		})

		Convey(`not found`, func() {
			_, err := fetch("inv")
			So(err, ShouldErrLike, `"invocations/inv" not found`)
		})

		insertInv := func(id span.InvocationID, finalizeTime time.Time, included ...span.InvocationID) []*spanner.Mutation {
			values := map[string]interface{}{
				"InvocationId":                      id,
				"State":                             pb.Invocation_COMPLETED,
				"Realm":                             "",
				"UpdateToken":                       "",
				"InvocationExpirationTime":          t3,
				"InvocationExpirationWeek":          t3,
				"ExpectedTestResultsExpirationTime": t3,
				"ExpectedTestResultsExpirationWeek": t3,
				"CreateTime":                        t0,
				"Deadline":                          finalizeTime,
				"FinalizeTime":                      finalizeTime,
			}
			ms := []*spanner.Mutation{span.InsertMap("Invocations", values)}
			for _, incl := range included {
				ms = append(ms, testutil.InsertInclusion(id, incl, ""))
			}
			return ms
		}

		Convey(`a -> []`, func() {
			testutil.MustApply(ctx, insertInv("a", t1)...)
			assertGraph(mustFetch("a"), `
				a ->
			`)
		})

		Convey(`a -> [b, c]`, func() {
			testutil.MustApply(ctx, insertInv("b", t1)...) // ready
			testutil.MustApply(ctx, insertInv("c", t2)...) // unready
			testutil.MustApply(ctx, insertInv("a", t2, "b", "c")...)
			assertGraph(mustFetch("a"), `
				a -> b c
				b ->
				c ->
			`)
		})

		Convey(`a -> b -> c`, func() {
			testutil.MustApply(ctx, insertInv("c", t2)...)
			testutil.MustApply(ctx, insertInv("b", t1, "c")...)
			testutil.MustApply(ctx, insertInv("a", t0, "b")...)
			assertGraph(mustFetch("a"), `
				a -> b
				b -> c
				c ->
			`)
		})
	})
}
