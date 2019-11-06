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
	"fmt"
	"io"
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

func (g *Graph) Dump(w io.Writer) {
	ids := make([]span.InvocationID, 0, len(g.Nodes))
	for _, n := range g.Nodes {
		ids = append(ids, n.ID)
	}
	span.SortInvocationIDs(ids)

	for _, id := range ids {
		n := g.Nodes[id]
		fmt.Fprintf(w, "%s ->", n.ID)

		keys := make([]span.InvocationID, 0, len(n.OutgoingEdges))
		for id := range n.OutgoingEdges {
			keys = append(keys, id)
		}
		span.SortInvocationIDs(keys)
		for _, key := range keys {
			fmt.Fprint(w, " ")
			fmt.Fprint(w, key)
		}
		fmt.Fprintln(w)
	}
}

var (
	t0 = testclock.TestRecentTimeUTC
	t1 = t0.Add(time.Hour)
	t2 = t1.Add(time.Hour)
	t3 = t2.Add(time.Hour)
)

func insertInv(id span.InvocationID, finalizeTime time.Time, included ...span.InvocationID) []*spanner.Mutation {
	ms := []*spanner.Mutation{span.InsertMap("Invocations", map[string]interface{}{
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
	})}
	for _, incl := range included {
		ms = append(ms, testutil.InsertInclusion(id, incl, ""))
	}
	return ms
}
func TestFetchGraph(t *testing.T) {
	Convey(`TestInclude`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		fetch := func(stabilizedOnly bool, roots ...span.InvocationID) (*Graph, error) {
			txn := span.Client(ctx).ReadOnlyTransaction()
			defer txn.Close()
			return FetchGraph(ctx, txn, stabilizedOnly, roots...)
		}

		mustFetch := func(stabilizedOnly bool, roots ...span.InvocationID) *Graph {
			g, err := fetch(stabilizedOnly, roots...)
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
			assertGraph(mustFetch(false), ``)
		})

		Convey(`not found`, func() {
			_, err := fetch(false, "inv")
			So(err, ShouldErrLike, `"invocations/inv" not found`)
		})

		Convey(`a -> []`, func() {
			testutil.MustApply(ctx, insertInv("a", t1)...)
			assertGraph(mustFetch(false, "a"), `
				a ->
			`)
		})

		Convey(`a -> [b, c]`, func() {
			testutil.MustApply(ctx, insertInv("b", t1)...) // stabilized
			testutil.MustApply(ctx, insertInv("c", t2)...) // not stabilized
			testutil.MustApply(ctx, insertInv("a", t2, "b", "c")...)
			assertGraph(mustFetch(false, "a"), `
				a -> b c
				b ->
				c ->
			`)
		})

		Convey(`a -> b -> c`, func() {
			testutil.MustApply(ctx, insertInv("c", t2)...)
			testutil.MustApply(ctx, insertInv("b", t1, "c")...)
			testutil.MustApply(ctx, insertInv("a", t0, "b")...)
			assertGraph(mustFetch(false, "a"), `
				a -> b
				b -> c
				c ->
			`)
		})

		Convey(`only stabilized`, func() {
			testutil.MustApply(ctx, insertInv("c", t2)...)
			testutil.MustApply(ctx, insertInv("b", t0)...)
			testutil.MustApply(ctx, insertInv("a", t1, "b", "c")...)
			assertGraph(mustFetch(true, "a"), `
				a -> b
				b ->
			`)
		})
	})
}

// BenchmarkChainFetch measures performance of a fetching a graph
// with a 10 linear inclusions.
func BenchmarkChainFetch(b *testing.B) {
	ctx := testutil.SpannerTestContext(b)
	client := span.Client(ctx)

	var ms []*spanner.Mutation
	var prev span.InvocationID
	for i := 0; i < 10; i++ {
		var included []span.InvocationID
		if prev != "" {
			included = append(included, prev)
		}
		id := span.InvocationID(fmt.Sprintf("inv%d", i))
		prev = id
		ms = append(ms, insertInv(id, t1, included...)...)
	}

	if _, err := client.Apply(ctx, ms); err != nil {
		b.Fatal(err)
	}

	fetch := func() {
		txn := span.Client(ctx).ReadOnlyTransaction()
		defer txn.Close()
		_, err := FetchGraph(ctx, txn, false, prev)
		if err != nil {
			b.Fatal(err)
		}
	}

	// Run fetch a few times before starting measuring.
	for i := 0; i < 5; i++ {
		fetch()
	}

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		fetch()
	}
}
