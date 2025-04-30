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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestMutateInvocation(t *testing.T) {
	ftt.Run("MayMutateInvocation", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		mayMutate := func(id invocations.ID) error {
			_, err := mutateInvocation(ctx, id, func(ctx context.Context) error {
				return nil
			})
			return err
		}

		t.Run("no token", func(t *ftt.Test) {
			err := mayMutate("inv")
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.Unauthenticated))
			assert.Loosely(t, err, should.ErrLike(`missing update-token metadata value`))
		})

		t.Run("with token", func(t *ftt.Test) {
			token, err := generateInvocationToken(ctx, "inv")
			assert.Loosely(t, err, should.BeNil)
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token))

			t.Run(`no invocation`, func(t *ftt.Test) {
				err := mayMutate("inv")
				as, ok := appstatus.Get(err)
				assert.That(t, ok, should.BeTrue)
				assert.That(t, as.Code(), should.Equal(codes.NotFound))
				assert.That(t, as.Message(), should.ContainSubstring(`invocations/inv not found`))
			})

			t.Run(`with finalized invocation`, func(t *ftt.Test) {
				testutil.MustApply(ctx, t, insert.Invocation("inv", pb.Invocation_FINALIZED, nil))
				err := mayMutate("inv")
				assert.Loosely(t, appstatus.Code(err), should.Equal(codes.FailedPrecondition))
				assert.Loosely(t, err, should.ErrLike(`invocations/inv is not active`))
			})

			t.Run(`with active invocation and different token`, func(t *ftt.Test) {
				testutil.MustApply(ctx, t, insert.Invocation("inv2", pb.Invocation_ACTIVE, nil))
				err := mayMutate("inv2")
				assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike(`invalid update token`))
			})

			t.Run(`with active invocation and same token`, func(t *ftt.Test) {
				testutil.MustApply(ctx, t, insert.Invocation("inv", pb.Invocation_ACTIVE, nil))
				err := mayMutate("inv")
				assert.Loosely(t, err, should.BeNil)
			})
		})
	})
}

func TestReadInvocation(t *testing.T) {
	ftt.Run(`ReadInvocationFull`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ct := testclock.TestRecentTimeUTC

		readInv := func() *pb.Invocation {
			ctx, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()

			inv, err := invocations.Read(ctx, "inv", invocations.AllFields)
			assert.Loosely(t, err, should.BeNil)
			return inv
		}

		t.Run(`Finalized`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, insert.Invocation("inv", pb.Invocation_FINALIZED, map[string]any{
				"CreateTime":        ct,
				"Deadline":          ct.Add(time.Hour),
				"FinalizeStartTime": ct.Add(2 * time.Hour),
				"FinalizeTime":      ct.Add(3 * time.Hour),
			}))

			inv := readInv()
			expected := &pb.Invocation{
				Name:                   "invocations/inv",
				State:                  pb.Invocation_FINALIZED,
				CreateTime:             pbutil.MustTimestampProto(ct),
				Deadline:               pbutil.MustTimestampProto(ct.Add(time.Hour)),
				FinalizeStartTime:      pbutil.MustTimestampProto(ct.Add(2 * time.Hour)),
				FinalizeTime:           pbutil.MustTimestampProto(ct.Add(3 * time.Hour)),
				Realm:                  insert.TestRealm,
				TestResultVariantUnion: &pb.Variant{},
			}
			assert.Loosely(t, inv, should.Match(expected))

			t.Run(`with included invocations`, func(t *ftt.Test) {
				testutil.MustApply(ctx, t,
					insert.Invocation("included0", pb.Invocation_FINALIZED, nil),
					insert.Invocation("included1", pb.Invocation_FINALIZED, nil),
					insert.Inclusion("inv", "included0"),
					insert.Inclusion("inv", "included1"),
				)

				inv := readInv()
				assert.Loosely(t, inv.IncludedInvocations, should.Match([]string{"invocations/included0", "invocations/included1"}))
			})
		})
	})
}
