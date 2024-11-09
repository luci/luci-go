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
}
