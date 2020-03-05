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

package spantest

import (
	"fmt"
	"testing"
	"time"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/clock"

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestReadInvocationFull(t *testing.T) {
	Convey(`ReadInvocationFull`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		start := clock.Now(ctx).UTC()

		// Insert some Invocations.
		testutil.MustApply(ctx,
			testutil.InsertInvocation("including", pb.Invocation_ACTIVE, map[string]interface{}{
				"CreateTime": start,
				"Deadline":   start.Add(time.Hour),
			}),
			testutil.InsertInvocation("included0", pb.Invocation_FINALIZED, nil),
			testutil.InsertInvocation("included1", pb.Invocation_FINALIZED, nil),
			testutil.InsertInclusion("including", "included0"),
			testutil.InsertInclusion("including", "included1"),
		)

		txn := span.Client(ctx).ReadOnlyTransaction()
		defer txn.Close()

		// Fetch back the top-level Invocation.
		inv, err := span.ReadInvocationFull(ctx, txn, "including")
		So(err, ShouldBeNil)
		So(inv, ShouldResembleProto, &pb.Invocation{
			Name:                "invocations/including",
			State:               pb.Invocation_ACTIVE,
			CreateTime:          pbutil.MustTimestampProto(start),
			Deadline:            pbutil.MustTimestampProto(start.Add(time.Hour)),
			IncludedInvocations: []string{"invocations/included0", "invocations/included1"},
		})
	})
}

func TestReadReachableInvocations(t *testing.T) {
	Convey(`TestInclude`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		insertInv := testutil.InsertFinalizedInvocationWithInclusions

		read := func(limit int, roots ...span.InvocationID) (span.InvocationIDSet, error) {
			txn := span.Client(ctx).ReadOnlyTransaction()
			defer txn.Close()
			return span.ReadReachableInvocations(ctx, txn, limit, span.NewInvocationIDSet(roots...))
		}

		mustReadIDs := func(limit int, roots ...span.InvocationID) span.InvocationIDSet {
			invs, err := read(limit, roots...)
			So(err, ShouldBeNil)
			return invs
		}

		Convey(`a -> []`, func() {
			testutil.MustApply(ctx, insertInv("a")...)
			So(mustReadIDs(100, "a"), ShouldResemble, span.NewInvocationIDSet("a"))
		})

		Convey(`a -> [b, c]`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				insertInv("a", "b", "c"),
				insertInv("b"),
				insertInv("c"),
			)...)
			So(mustReadIDs(100, "a"), ShouldResemble, span.NewInvocationIDSet("a", "b", "c"))
		})

		Convey(`a -> b -> c`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				insertInv("a", "b"),
				insertInv("b", "c"),
				insertInv("c"),
			)...)
			So(mustReadIDs(100, "a"), ShouldResemble, span.NewInvocationIDSet("a", "b", "c"))
		})

		Convey(`limit`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				insertInv("a", "b"),
				insertInv("b", "c"),
				insertInv("c"),
			)...)
			_, err := read(1, "a")
			So(err, ShouldNotBeNil)
			So(span.TooManyInvocationsTag.In(err), ShouldBeTrue)
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
		ms = append(ms, testutil.InsertFinalizedInvocationWithInclusions(id, included...)...)
	}

	if _, err := client.Apply(ctx, ms); err != nil {
		b.Fatal(err)
	}

	read := func() {
		txn := span.Client(ctx).ReadOnlyTransaction()
		defer txn.Close()

		if _, err := span.ReadReachableInvocations(ctx, txn, 100, span.NewInvocationIDSet(prev)); err != nil {
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

func TestQueryInvocations(t *testing.T) {
	Convey(`TestQueryInvocations`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		testutil.MustApply(ctx,
			testutil.InsertInvocation("inv0", pb.Invocation_FINALIZED, nil),
			testutil.InsertInvocation("inv1", pb.Invocation_FINALIZED, nil),
			testutil.InsertInvocation("inv2", pb.Invocation_FINALIZED, nil),
		)

		txn := span.Client(ctx).ReadOnlyTransaction()
		defer txn.Close()

		Convey(`One name`, func() {
			invs, err := span.ReadInvocationsFull(ctx, txn, span.NewInvocationIDSet("inv1"))
			So(err, ShouldBeNil)
			So(invs, ShouldHaveLength, 1)
			So(invs, ShouldContainKey, span.InvocationID("inv1"))
			So(invs["inv1"].Name, ShouldEqual, "invocations/inv1")
			So(invs["inv1"].State, ShouldEqual, pb.Invocation_FINALIZED)
		})

		Convey(`Two names`, func() {
			invs, err := span.ReadInvocationsFull(ctx, txn, span.NewInvocationIDSet("inv0", "inv1"))
			So(err, ShouldBeNil)
			So(invs, ShouldHaveLength, 2)
			So(invs, ShouldContainKey, span.InvocationID("inv0"))
			So(invs, ShouldContainKey, span.InvocationID("inv1"))
			So(invs["inv0"].Name, ShouldEqual, "invocations/inv0")
			So(invs["inv0"].State, ShouldEqual, pb.Invocation_FINALIZED)
		})

		Convey(`Not found`, func() {
			_, err := span.ReadInvocationsFull(ctx, txn, span.NewInvocationIDSet("inv0", "x"))
			So(err, ShouldErrLike, `invocations/x not found`)
		})

	})
}
