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

package recorder

import (
	"context"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/clock/testclock"

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
	. "go.chromium.org/luci/resultdb/internal/testutil"
)

func TestMutateInvocation(t *testing.T) {
	Convey("MayMutateInvocation", t, func() {
		ctx := SpannerTestContext(t)
		ct := testclock.TestRecentTimeUTC

		mayMutate := func() error {
			return mutateInvocation(ctx, "inv", func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
				return nil
			})
		}

		Convey("no token", func() {
			err := mayMutate()
			So(err, ShouldHaveAppStatus, codes.Unauthenticated, `missing update-token metadata value`)
		})

		Convey("with token", func() {
			const token = "update token"
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(updateTokenMetadataKey, token))

			Convey(`no invocation`, func() {
				err := mayMutate()
				So(err, ShouldHaveAppStatus, codes.NotFound, `invocations/inv not found`)
			})

			Convey(`with finalized invocation`, func() {
				MustApply(ctx, InsertInvocation("inv", pb.Invocation_FINALIZED, ct, map[string]interface{}{"UpdateToken": token}))
				err := mayMutate()
				So(err, ShouldHaveAppStatus, codes.FailedPrecondition, `invocations/inv is not active`)
			})

			Convey(`with active invocation and different token`, func() {
				MustApply(ctx, InsertInvocation("inv", pb.Invocation_ACTIVE, ct, map[string]interface{}{"UpdateToken": "different token"}))
				err := mayMutate()
				So(err, ShouldHaveAppStatus, codes.PermissionDenied, `invalid update token`)
			})

			Convey(`with active invocation and same token`, func() {
				MustApply(ctx, InsertInvocation("inv", pb.Invocation_ACTIVE, ct, map[string]interface{}{"UpdateToken": token}))

				err := mayMutate()
				So(err, ShouldBeNil)
			})
		})
	})
}

func TestReadInvocation(t *testing.T) {
	Convey(`ReadInvocationFull`, t, func() {
		ctx := SpannerTestContext(t)
		ct := testclock.TestRecentTimeUTC

		readInv := func() *pb.Invocation {
			txn := span.Client(ctx).ReadOnlyTransaction()
			defer txn.Close()

			inv, err := span.ReadInvocationFull(ctx, txn, "inv")
			So(err, ShouldBeNil)
			return inv
		}

		Convey(`Finalized`, func() {
			MustApply(ctx, InsertInvocation("inv", pb.Invocation_FINALIZED, ct, nil))

			inv := readInv()
			expected := &pb.Invocation{
				Name:         "invocations/inv",
				State:        pb.Invocation_FINALIZED,
				CreateTime:   pbutil.MustTimestampProto(ct),
				Deadline:     pbutil.MustTimestampProto(ct.Add(time.Hour)),
				FinalizeTime: pbutil.MustTimestampProto(ct.Add(time.Hour)),
			}
			So(inv, ShouldResembleProto, expected)

			Convey(`with included invocations`, func() {
				MustApply(ctx,
					InsertInvocation("included0", pb.Invocation_FINALIZED, ct, nil),
					InsertInvocation("included1", pb.Invocation_FINALIZED, ct, nil),
					InsertInclusion("inv", "included0"),
					InsertInclusion("inv", "included1"),
				)

				inv := readInv()
				So(inv.IncludedInvocations, ShouldResemble, []string{"invocations/included0", "invocations/included1"})
			})
		})
	})
}
