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

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestMutateInvocation(t *testing.T) {
	Convey("MayMutateInvocation", t, func() {
		ctx := testutil.SpannerTestContext(t)

		mayMutate := func(id invocations.ID) error {
			return mutateInvocation(ctx, id, func(ctx context.Context) error {
				return nil
			})
		}

		Convey("no token", func() {
			err := mayMutate("inv")
			So(err, ShouldHaveAppStatus, codes.Unauthenticated, `missing update-token metadata value`)
		})

		Convey("with token", func() {
			token, err := generateInvocationToken(ctx, "inv")
			So(err, ShouldBeNil)
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token))

			Convey(`no invocation`, func() {
				err := mayMutate("inv")
				So(err, ShouldHaveAppStatus, codes.NotFound, `invocations/inv not found`)
			})

			Convey(`with finalized invocation`, func() {
				testutil.MustApply(ctx, insert.Invocation("inv", pb.Invocation_FINALIZED, nil))
				err := mayMutate("inv")
				So(err, ShouldHaveAppStatus, codes.FailedPrecondition, `invocations/inv is not active`)
			})

			Convey(`with active invocation and different token`, func() {
				testutil.MustApply(ctx, insert.Invocation("inv2", pb.Invocation_ACTIVE, nil))
				err := mayMutate("inv2")
				So(err, ShouldHaveAppStatus, codes.PermissionDenied, `invalid update token`)
			})

			Convey(`with active invocation and same token`, func() {
				testutil.MustApply(ctx, insert.Invocation("inv", pb.Invocation_ACTIVE, nil))
				err := mayMutate("inv")
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
			ctx, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()

			inv, err := invocations.Read(ctx, "inv")
			So(err, ShouldBeNil)
			return inv
		}

		Convey(`Finalized`, func() {
			testutil.MustApply(ctx, insert.Invocation("inv", pb.Invocation_FINALIZED, map[string]any{
				"CreateTime":        ct,
				"Deadline":          ct.Add(time.Hour),
				"FinalizeStartTime": ct.Add(2 * time.Hour),
				"FinalizeTime":      ct.Add(3 * time.Hour),
			}))

			inv := readInv()
			expected := &pb.Invocation{
				Name:              "invocations/inv",
				State:             pb.Invocation_FINALIZED,
				CreateTime:        pbutil.MustTimestampProto(ct),
				Deadline:          pbutil.MustTimestampProto(ct.Add(time.Hour)),
				FinalizeStartTime: pbutil.MustTimestampProto(ct.Add(2 * time.Hour)),
				FinalizeTime:      pbutil.MustTimestampProto(ct.Add(3 * time.Hour)),
				Realm:             insert.TestRealm,
			}
			So(inv, ShouldResembleProto, expected)

			Convey(`with included invocations`, func() {
				testutil.MustApply(ctx,
					insert.Invocation("included0", pb.Invocation_FINALIZED, nil),
					insert.Invocation("included1", pb.Invocation_FINALIZED, nil),
					insert.Inclusion("inv", "included0"),
					insert.Inclusion("inv", "included1"),
				)

				inv := readInv()
				So(inv.IncludedInvocations, ShouldResemble, []string{"invocations/included0", "invocations/included1"})
			})
		})
	})
}
