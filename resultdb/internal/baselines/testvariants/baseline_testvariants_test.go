// Copyright 2023 The LUCI Authors.
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

package baselinetestvariant

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/testutil"
)

func TestRead(t *testing.T) {
	ftt.Run(`Invalid`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		t.Run(`Not Found`, func(t *ftt.Test) {
			_, err := Read(span.Single(ctx), "chromium", "try:linux-rel", "ninja://some/test:test_id", "12345")
			assert.Loosely(t, err, should.ErrLike(NotFound))
		})
	})

	ftt.Run(`Valid`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		t.Run(`Exists`, func(t *ftt.Test) {
			exp := &BaselineTestVariant{
				Project:     "chromium",
				BaselineID:  "try:linux-rel",
				TestID:      "ninja://some/test:test_id",
				VariantHash: "12345",
			}
			commitTime := testutil.MustApply(ctx, t, Create(exp.Project, exp.BaselineID, exp.TestID, exp.VariantHash))

			res, err := Read(span.Single(ctx), exp.Project, exp.BaselineID, exp.TestID, exp.VariantHash)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.Project, should.Equal(exp.Project))
			assert.Loosely(t, res.BaselineID, should.Equal(exp.BaselineID))
			assert.Loosely(t, res.TestID, should.Equal(exp.TestID))
			assert.Loosely(t, res.VariantHash, should.Equal(exp.VariantHash))
			assert.Loosely(t, res.LastUpdated, should.Match(commitTime))
		})
	})
}
