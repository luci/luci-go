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

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/server/span"

	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTestResultCount(t *testing.T) {
	Convey(`TestRead`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		testutil.MustApply(ctx, insert.Invocation("inv", pb.Invocation_FINALIZED, nil))

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
		So(err, ShouldBeNil)

		count, err := ReadTestResultCount(span.Single(ctx), invocations.NewIDSet(invID))
		So(count, ShouldEqual, 30)
		So(err, ShouldBeNil)
	})
}
