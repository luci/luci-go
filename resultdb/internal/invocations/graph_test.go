// Copyright 2020 The LUCI Authors.
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

package invocations

import (
	"fmt"
	"testing"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/internal/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func TestReachable(t *testing.T) {
	Convey(`Reachable`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		node := insertInvocationIncluding

		read := func(roots ...ID) (IDSet, error) {
			txn := span.Client(ctx).ReadOnlyTransaction()
			defer txn.Close()
			return Reachable(ctx, txn, NewIDSet(roots...))
		}

		mustReadIDs := func(roots ...ID) IDSet {
			invs, err := read(roots...)
			So(err, ShouldBeNil)
			return invs
		}

		Convey(`a -> []`, func() {
			testutil.MustApply(ctx, node("a")...)
			So(mustReadIDs("a"), ShouldResemble, NewIDSet("a"))
		})

		Convey(`a -> [b, c]`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				node("a", "b", "c"),
				node("b"),
				node("c"),
			)...)
			So(mustReadIDs("a"), ShouldResemble, NewIDSet("a", "b", "c"))
		})

		Convey(`a -> b -> c`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				node("a", "b"),
				node("b", "c"),
				node("c"),
			)...)
			So(mustReadIDs("a"), ShouldResemble, NewIDSet("a", "b", "c"))
		})
	})
}

// BenchmarkChainFetch measures performance of a fetching a graph
// with a 10 linear inclusions.
func BenchmarkChainFetch(b *testing.B) {
	ctx := testutil.SpannerTestContext(b)
	client := span.Client(ctx)

	var ms []*spanner.Mutation
	var prev ID
	for i := 0; i < 10; i++ {
		var included []ID
		if prev != "" {
			included = append(included, prev)
		}
		id := ID(fmt.Sprintf("inv%d", i))
		prev = id
		ms = append(ms, insertInvocationIncluding(id, included...)...)
	}

	if _, err := client.Apply(ctx, ms); err != nil {
		b.Fatal(err)
	}

	read := func() {
		txn := span.Client(ctx).ReadOnlyTransaction()
		defer txn.Close()

		if _, err := Reachable(ctx, txn, NewIDSet(prev)); err != nil {
			b.Fatal(err)
		}
	}

	// Run fetch a few times before starting measuring.
	for i := 0; i < 5; i++ {
		read()
	}

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		read()
	}
}
