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

package resultcount

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestTestResultCount(t *testing.T) {
	ftt.Run(`TestRead`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		testutil.MustApply(ctx, t, insert.Invocation("inv", pb.Invocation_FINALIZED, nil))

		invID := invocations.ID("inv")
		_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
			if err := IncrementTestResultCount(ctx, invID, 10); err != nil {
				return err
			}
			if err := IncrementTestResultCount(ctx, invID, 20); err != nil {
				return err
			}

			return nil
		})
		assert.Loosely(t, err, should.BeNil)

		count, err := ReadTestResultCount(span.Single(ctx), invocations.NewIDSet(invID))
		assert.Loosely(t, count, should.Equal(30))
		assert.Loosely(t, err, should.BeNil)
	})
	ftt.Run(`BatchIncrementTestResultCount`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		testutil.MustApply(ctx, t,
			insert.Invocation("inv1", pb.Invocation_FINALIZED, nil),
			insert.Invocation("inv2", pb.Invocation_FINALIZED, nil),
			insert.Invocation("inv3", pb.Invocation_FINALIZED, nil),
		)

		for i := 1; i <= 10; i++ {
			deltas := map[invocations.ID]int64{
				"inv1": 10,
				"inv2": 20,
				"inv3": 0, // Should be ignored
			}

			_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
				return BatchIncrementTestResultCount(ctx, deltas)
			})
			if err != nil {
				err = errors.Fmt("%v: %w", i, err)
			}
			assert.Loosely(t, err, should.BeNil)

			// Verify counts after first batch increment
			count1, err := ReadTestResultCount(span.Single(ctx), invocations.NewIDSet("inv1"))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, count1, should.Equal(10*i))

			count2, err := ReadTestResultCount(span.Single(ctx), invocations.NewIDSet("inv2"))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, count2, should.Equal(20*i))

			count3, err := ReadTestResultCount(span.Single(ctx), invocations.NewIDSet("inv3"))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, count3, should.Equal(0*i))
		}
	})
}
