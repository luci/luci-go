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

		now := clock.Now(ctx)

		// Insert some Invocations.
		testutil.MustApply(ctx,
			testutil.InsertInvocation("including", pb.Invocation_ACTIVE, "", now),
			testutil.InsertInvocation("included0", pb.Invocation_COMPLETED, "", now),
			testutil.InsertInvocation("included1", pb.Invocation_COMPLETED, "", now),
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
			CreateTime:          pbutil.MustTimestampProto(now),
			Deadline:            pbutil.MustTimestampProto(now.Add(time.Hour)),
			IncludedInvocations: []string{"invocations/included0", "invocations/included1"},
		})
	})
}

func TestReadReachableInvocations(t *testing.T) {
	Convey(`TestInclude`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		insertInv := testutil.InsertInvocationWithInclusions

		read := func(limit int, rootIDs ...span.InvocationID) (map[span.InvocationID]*pb.Invocation, error) {
			txn := span.Client(ctx).ReadOnlyTransaction()
			defer txn.Close()

			roots := make(map[span.InvocationID]*pb.Invocation, len(rootIDs))
			for _, id := range rootIDs {
				root, err := span.ReadInvocationFull(ctx, txn, id)
				So(err, ShouldBeNil)
				roots[id] = root
			}
			return span.ReadReachableInvocations(ctx, txn, limit, roots)
		}

		mustReadIDs := func(limit int, roots ...span.InvocationID) []span.InvocationID {
			invs, err := read(limit, roots...)
			So(err, ShouldBeNil)
			ids := make([]span.InvocationID, 0, len(invs))
			for id := range invs {
				ids = append(ids, id)
			}
			span.SortInvocationIDs(ids)
			return ids
		}

		Convey(`a -> []`, func() {
			testutil.MustApply(ctx, insertInv("a")...)
			So(mustReadIDs(100, "a"), ShouldResemble, []span.InvocationID{"a"})
		})

		Convey(`a -> [b, c]`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				insertInv("a", "b", "c"),
				insertInv("b"),
				insertInv("c"),
			)...)
			So(mustReadIDs(100, "a"), ShouldResemble, []span.InvocationID{"a", "b", "c"})
		})

		Convey(`a -> b -> c`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				insertInv("a", "b"),
				insertInv("b", "c"),
				insertInv("c"),
			)...)
			So(mustReadIDs(100, "a"), ShouldResemble, []span.InvocationID{"a", "b", "c"})
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
		ms = append(ms, testutil.InsertInvocationWithInclusions(id, included...)...)
	}

	if _, err := client.Apply(ctx, ms); err != nil {
		b.Fatal(err)
	}

	rootInvTxn := span.Client(ctx).ReadOnlyTransaction()
	defer rootInvTxn.Close()
	root, err := span.ReadInvocationFull(ctx, rootInvTxn, prev)
	So(err, ShouldBeNil)
	roots := map[span.InvocationID]*pb.Invocation{prev: root}

	read := func() {
		txn := span.Client(ctx).ReadOnlyTransaction()
		defer txn.Close()

		_, err = span.ReadReachableInvocations(ctx, txn, 100, roots)
		if err != nil {
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
		now := clock.Now(ctx)

		query := func(pred *pb.InvocationPredicate) map[span.InvocationID]*pb.Invocation {
			txn := span.Client(ctx).ReadOnlyTransaction()
			defer txn.Close()
			invs, err := span.QueryInvocations(ctx, txn, pred, 100)
			So(err, ShouldBeNil)
			return invs
		}

		Convey(`No graph traversal`, func() {
			tags := pbutil.StringPairs("a", "b", "a", "c")
			testutil.MustApply(ctx,
				testutil.InsertInvocation("inv0", pb.Invocation_COMPLETED, "", now),
				testutil.InsertInvocation("inv1", pb.Invocation_COMPLETED, "", now),
				testutil.InsertInvocation("inv2", pb.Invocation_COMPLETED, "", now),
				span.InsertMap("InvocationsByTag", map[string]interface{}{
					"TagId":        span.TagRowID(tags[0]),
					"InvocationId": span.InvocationID("inv0"),
				}),
				span.InsertMap("InvocationsByTag", map[string]interface{}{
					"TagId":        span.TagRowID(tags[0]),
					"InvocationId": span.InvocationID("inv1"),
				}),
				span.InsertMap("InvocationsByTag", map[string]interface{}{
					"TagId":        span.TagRowID(tags[1]),
					"InvocationId": span.InvocationID("inv1"),
				}),
				span.InsertMap("InvocationsByTag", map[string]interface{}{
					"TagId":        span.TagRowID(tags[1]),
					"InvocationId": span.InvocationID("inv2"),
				}),
			)

			txn := span.Client(ctx).ReadOnlyTransaction()
			defer txn.Close()

			Convey(`Search by Name`, func() {
				pred := &pb.InvocationPredicate{Names: []string{"invocations/inv1"}}
				Convey(`One name`, func() {
					invs, err := span.QueryInvocations(ctx, txn, pred, 10)
					So(err, ShouldBeNil)
					So(invs, ShouldHaveLength, 1)
					So(invs, ShouldContainKey, span.InvocationID("inv1"))
					So(invs["inv1"].Name, ShouldEqual, "invocations/inv1")
					So(invs["inv1"].State, ShouldEqual, pb.Invocation_COMPLETED)
				})

				Convey(`Two names`, func() {
					pred := &pb.InvocationPredicate{Names: []string{"invocations/inv0", "invocations/inv1"}}
					invs, err := span.QueryInvocations(ctx, txn, pred, 10)
					So(err, ShouldBeNil)
					So(invs, ShouldHaveLength, 2)
					So(invs, ShouldContainKey, span.InvocationID("inv0"))
					So(invs, ShouldContainKey, span.InvocationID("inv1"))
					So(invs["inv0"].Name, ShouldEqual, "invocations/inv0")
					So(invs["inv0"].State, ShouldEqual, pb.Invocation_COMPLETED)
				})
			})

			Convey(`Search by Tags`, func() {
				Convey(`One tag`, func() {
					pred := &pb.InvocationPredicate{Tags: tags[:1]}
					invs, err := span.QueryInvocations(ctx, txn, pred, 10)
					So(err, ShouldBeNil)
					So(invs, ShouldHaveLength, 2)
					So(invs, ShouldContainKey, span.InvocationID("inv0"))
					So(invs, ShouldContainKey, span.InvocationID("inv1"))
					So(invs["inv0"].Name, ShouldEqual, "invocations/inv0")
					So(invs["inv0"].State, ShouldEqual, pb.Invocation_COMPLETED)
				})

				Convey(`Two tags`, func() {
					pred := &pb.InvocationPredicate{Tags: tags}
					invs, err := span.QueryInvocations(ctx, txn, pred, 10)
					So(err, ShouldBeNil)
					So(invs, ShouldHaveLength, 3)
					So(invs, ShouldContainKey, span.InvocationID("inv0"))
					So(invs, ShouldContainKey, span.InvocationID("inv1"))
					So(invs, ShouldContainKey, span.InvocationID("inv2"))
					So(invs["inv0"].Name, ShouldEqual, "invocations/inv0")
					So(invs["inv0"].State, ShouldEqual, pb.Invocation_COMPLETED)
				})
			})

			Convey(`Search by Tags and Names, with duplicates`, func() {
				pred := &pb.InvocationPredicate{
					Names: []string{"invocations/inv0"},
					Tags:  tags[:1],
				}
				invs, err := span.QueryInvocations(ctx, txn, pred, 10)
				So(err, ShouldBeNil)
				So(invs, ShouldHaveLength, 2)
				So(invs, ShouldContainKey, span.InvocationID("inv0"))
				So(invs, ShouldContainKey, span.InvocationID("inv1"))
			})

			Convey(`limit`, func() {
				pred := &pb.InvocationPredicate{Tags: tags}
				_, err := span.QueryInvocations(ctx, txn, pred, 1)
				So(err, ShouldErrLike, `more than 1 invocations match the predicate`)
				So(span.TooManyInvocationsTag.In(err), ShouldBeTrue)
			})
		})
		Convey(`Invocations reachable from an invocation with a certain name`, func() {
			insertInv := testutil.InsertInvocationWithInclusions
			testutil.MustApply(ctx, testutil.CombineMutations(
				insertInv("a", "b", "c"),
				insertInv("b", "d"),
				insertInv("c"),
				insertInv("d", "e"),
				insertInv("e"),
				// unrelated invocations
				insertInv("x"),
				insertInv("y", "a"),
			)...)
			actual := query(&pb.InvocationPredicate{
				Names: []string{"invocations/a"},
			})
			So(actual, ShouldHaveLength, 5)
			So(actual, ShouldContainKey, span.InvocationID("a"))
			So(actual, ShouldContainKey, span.InvocationID("b"))
			So(actual, ShouldContainKey, span.InvocationID("c"))
			So(actual, ShouldContainKey, span.InvocationID("d"))
			So(actual, ShouldContainKey, span.InvocationID("e"))

			So(actual["a"].IncludedInvocations, ShouldResemble, []string{"invocations/b", "invocations/c"})
		})
	})
}
