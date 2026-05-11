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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/testutil"
)

func TestSaveTestResults(t *testing.T) {
	ftt.Run("SaveUnverified", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		result := NewTestResult().Build()
		_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
			ms := result.SaveUnverified()
			span.BufferWrite(ctx, ms)
			return nil
		})
		assert.Loosely(t, err, should.BeNil)

		results, err := ReadAllForTesting(span.Single(ctx))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, results, should.Match([]*TestResult{result}))
	})
}
