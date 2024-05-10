// Copyright 2024 The LUCI Authors.
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

package lowlatency

import (
	"context"
	"testing"

	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/testutil"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestSaveTestResults(t *testing.T) {
	Convey("SaveUnverified", t, func() {
		ctx := testutil.SpannerTestContext(t)

		result := NewTestResult().Build()
		_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
			ms := result.SaveUnverified()
			span.BufferWrite(ctx, ms)
			return nil
		})
		So(err, ShouldBeNil)

		results, err := ReadAllForTesting(span.Single(ctx))
		So(err, ShouldBeNil)
		So(results, ShouldResembleProto, []*TestResult{result})
	})
}
