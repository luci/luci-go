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

package main

import (
	"context"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestMutateInvocation(t *testing.T) {
	Convey("MayMutateInvocation", t, func() {
		ctx := testutil.SpannerTestContext(t)
		ct := testclock.TestRecentTimeUTC

		mayMutate := func() error {
			return mutateInvocation(ctx, "inv", func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
				return nil
			})
		}

		Convey("no token", func() {
			err := mayMutate()
			So(err, ShouldErrLike, `missing "update-token" metadata value`)
			So(grpcutil.Code(err), ShouldEqual, codes.Unauthenticated)
		})

		Convey("with token", func() {
			const token = "update token"
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(updateTokenMetadataKey, token))

			Convey(`no invocation`, func() {
				err := mayMutate()
				So(err, ShouldErrLike, `"invocations/inv" not found`)
				So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
			})

			Convey(`with finalized invocation`, func() {
				testutil.MustApply(ctx, testutil.InsertInvocation("inv", pb.Invocation_COMPLETED, token, ct))
				err := mayMutate()
				So(err, ShouldErrLike, `"invocations/inv" is not active`)
				So(grpcutil.Code(err), ShouldEqual, codes.FailedPrecondition)
			})

			Convey(`with active invocation and different token`, func() {
				testutil.MustApply(ctx, testutil.InsertInvocation("inv", pb.Invocation_ACTIVE, "different token", ct))
				err := mayMutate()
				So(err, ShouldErrLike, `invalid update token`)
				So(grpcutil.Code(err), ShouldEqual, codes.PermissionDenied)
			})

			Convey(`with exceeded deadline`, func() {
				testutil.MustApply(ctx, testutil.InsertInvocation("inv", pb.Invocation_ACTIVE, token, ct))

				// Mock now to be after deadline.
				clock.Get(ctx).(testclock.TestClock).Add(2 * time.Hour)

				err := mayMutate()
				So(err, ShouldErrLike, `"invocations/inv" is not active`)
				So(grpcutil.Code(err), ShouldEqual, codes.FailedPrecondition)

				// Confirm the invocation has been updated.
				var state pb.Invocation_State
				var ft time.Time
				inv := span.InvocationID("inv")
				testutil.MustReadRow(ctx, "Invocations", inv.Key(), map[string]interface{}{
					"State":        &state,
					"FinalizeTime": &ft,
				})
				So(state, ShouldEqual, pb.Invocation_INTERRUPTED)
				So(ft, ShouldEqual, ct.Add(time.Hour))
			})

			Convey(`with active invocation and same token`, func() {
				testutil.MustApply(ctx, testutil.InsertInvocation("inv", pb.Invocation_ACTIVE, token, ct))

				err := mayMutate()
				So(err, ShouldBeNil)
			})
		})
	})
}

func TestReadInvocation(t *testing.T) {
	Convey(`ReadInvocationFull`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		ct := testclock.TestRecentTimeUTC

		readInv := func() *pb.Invocation {
			txn := span.Client(ctx).ReadOnlyTransaction()
			defer txn.Close()

			inv, err := span.ReadInvocationFull(ctx, txn, "inv")
			So(err, ShouldBeNil)
			return inv
		}

		Convey(`completed`, func() {
			testutil.MustApply(ctx, testutil.InsertInvocation("inv", pb.Invocation_COMPLETED, "", ct))

			inv := readInv()
			expected := &pb.Invocation{
				Name:         "invocations/inv",
				State:        pb.Invocation_COMPLETED,
				CreateTime:   pbutil.MustTimestampProto(ct),
				Deadline:     pbutil.MustTimestampProto(ct.Add(time.Hour)),
				FinalizeTime: pbutil.MustTimestampProto(ct.Add(time.Hour)),
			}
			So(inv, ShouldResembleProto, expected)

			Convey(`with inclusions`, func() {
				testutil.MustApply(ctx,
					testutil.InsertInvocation("completed", pb.Invocation_COMPLETED, "", ct.Add(-time.Hour)),
					testutil.InsertInvocation("active", pb.Invocation_ACTIVE, "", ct),
					testutil.InsertInclusion("inv", "completed", "active"),
					testutil.InsertInclusion("inv", "active", ""),
				)

				inv := readInv()
				actual := &pb.Invocation{Inclusions: inv.Inclusions}
				So(actual, ShouldResembleProto, &pb.Invocation{
					Inclusions: map[string]*pb.Invocation_InclusionAttrs{
						"invocations/completed": {
							Stable:       true,
							OverriddenBy: "invocations/active",
						},
						"invocations/active": {},
					},
				})
			})
		})

		Convey(`active with inclusions`, func() {
			testutil.MustApply(ctx,
				testutil.InsertInvocation("inv", pb.Invocation_ACTIVE, "", ct),
				testutil.InsertInvocation("completed", pb.Invocation_COMPLETED, "", ct.Add(-time.Hour)),
				testutil.InsertInvocation("active", pb.Invocation_ACTIVE, "", ct),
				testutil.InsertInclusion("inv", "completed", "active"),
				testutil.InsertInclusion("inv", "active", ""),
			)

			inv := readInv()
			actual := &pb.Invocation{Inclusions: inv.Inclusions}
			So(actual, ShouldResembleProto, &pb.Invocation{
				Inclusions: map[string]*pb.Invocation_InclusionAttrs{
					"invocations/completed": {
						Stable:       true,
						OverriddenBy: "invocations/active",
					},
					"invocations/active": {},
				},
			})
		})
	})
}

func TestWriteInvocationByTags(t *testing.T) {
	Convey(`getInvocationsByTagMutations`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		muts := insertInvocationsByTag("inv1", pbutil.StringPairs("k1", "v11", "k1", "v12", "k2", "v2"))
		muts = append(muts, insertInvocationsByTag("inv2", pbutil.StringPairs("k1", "v11", "k3", "v3"))...)
		testutil.MustApply(ctx, muts...)

		test := func(invID span.InvocationID, k, v string) {
			key := spanner.Key{span.TagRowID(pbutil.StringPair(k, v)), invID.RowID()}
			var throwaway string // we need to read into *something*
			err := span.ReadRow(ctx, span.Client(ctx).Single(), "InvocationsByTag", key, map[string]interface{}{
				"InvocationId": &throwaway,
			})
			So(err, ShouldBeNil)
		}

		test("inv1", "k1", "v11")

		// duplicated key in same invocation
		test("inv1", "k1", "v12")

		// duplicated tag in different invocation
		test("inv2", "k1", "v11")

		test("inv1", "k2", "v2")
		test("inv2", "k3", "v3")
	})
}
