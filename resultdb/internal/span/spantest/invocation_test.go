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
	"go.chromium.org/luci/common/clock/testclock"

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

		read := func(roots ...span.InvocationID) (map[span.InvocationID]*pb.Invocation, error) {
			txn := span.Client(ctx).ReadOnlyTransaction()
			defer txn.Close()
			return span.ReadReachableInvocations(ctx, txn, roots...)
		}

		mustReadIDs := func(roots ...span.InvocationID) []span.InvocationID {
			invs, err := read(roots...)
			So(err, ShouldBeNil)
			ids := make([]span.InvocationID, 0, len(invs))
			for id := range invs {
				ids = append(ids, id)
			}
			span.SortInvocationIDs(ids)
			return ids
		}

		Convey(`fetch nothing`, func() {
			So(mustReadIDs(), ShouldBeEmpty)
		})

		Convey(`not found`, func() {
			_, err := read("inv")
			So(err, ShouldErrLike, `"invocations/inv" not found`)
		})

		Convey(`a -> []`, func() {
			testutil.MustApply(ctx, insertInv("a")...)
			So(mustReadIDs("a"), ShouldResemble, []span.InvocationID{"a"})
		})

		Convey(`a -> [b, c]`, func() {
			testutil.MustApply(ctx, insertInv("b")...)
			testutil.MustApply(ctx, insertInv("c")...)
			testutil.MustApply(ctx, insertInv("a", "b", "c")...)
			So(mustReadIDs("a"), ShouldResemble, []span.InvocationID{"a", "b", "c"})
		})

		Convey(`a -> b -> c`, func() {
			testutil.MustApply(ctx, insertInv("c")...)
			testutil.MustApply(ctx, insertInv("b", "c")...)
			testutil.MustApply(ctx, insertInv("a", "b")...)
			So(mustReadIDs("a"), ShouldResemble, []span.InvocationID{"a", "b", "c"})
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
		ms = append(ms, insertInv(id, included...)...)
	}

	if _, err := client.Apply(ctx, ms); err != nil {
		b.Fatal(err)
	}

	read := func() {
		txn := span.Client(ctx).ReadOnlyTransaction()
		defer txn.Close()
		_, err := span.ReadReachableInvocations(ctx, txn, prev)
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

func insertInv(id span.InvocationID, included ...span.InvocationID) []*spanner.Mutation {
	t := testclock.TestRecentTimeUTC
	ms := []*spanner.Mutation{span.InsertMap("Invocations", map[string]interface{}{
		"InvocationId":                      id,
		"ShardId":                           0,
		"State":                             pb.Invocation_COMPLETED,
		"Realm":                             "",
		"UpdateToken":                       "",
		"InvocationExpirationTime":          t,
		"ExpectedTestResultsExpirationTime": t,
		"CreateTime":                        t,
		"Deadline":                          t,
		"FinalizeTime":                      t,
	})}
	for _, incl := range included {
		ms = append(ms, testutil.InsertInclusion(id, incl))
	}
	return ms
}
