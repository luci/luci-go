// Copyright 2026 The LUCI Authors.
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
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/testutil"
	"go.chromium.org/luci/analysis/pbutil"
)

func TestQuerySourceVerdicts(t *testing.T) {
	ftt.Run("QuerySourceVerdicts", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		// Insert test data.
		testutil.MustApply(ctx, t, CreateTestData()...)

		q := SourceVerdictQuery{
			Project:          "test-project",
			TestID:           "test-id",
			VariantHash:      pbutil.VariantHash(pbutil.Variant("key", "value")),
			RefHash:          pbutil.SourceRefHash(TestRef),
			AllowedSubrealms: []string{"testrealm", "otherrealm"},
			PageSize:         100,
		}

		t.Run("Basic", func(t *ftt.Test) {
			results, _, err := q.Fetch(span.Single(ctx), PageToken{})
			assert.Loosely(t, err, should.BeNil)

			expected := ExpectedSourceVerdicts()
			assert.Loosely(t, results, should.Match(expected))
		})

		t.Run("With pagination", func(t *ftt.Test) {
			q.PageSize = 5

			expected := ExpectedSourceVerdicts()

			// Page 1
			results, nextPageToken, err := q.Fetch(span.Single(ctx), PageToken{})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, results, should.Match(expected[:5]))
			assert.Loosely(t, nextPageToken.LastSourcePosition, should.Equal(96))

			// Page 2
			results, nextPageToken, err = q.Fetch(span.Single(ctx), nextPageToken)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, results, should.Match(expected[5:10]))
			assert.Loosely(t, nextPageToken.LastSourcePosition, should.Equal(91))

			// Page 3
			results, nextPageToken, err = q.Fetch(span.Single(ctx), nextPageToken)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, results, should.BeEmpty)
			assert.Loosely(t, nextPageToken, should.Match(PageToken{}))
		})

		t.Run("With filter", func(t *ftt.Test) {
			expected := FilterSourceVerdicts(ExpectedSourceVerdicts(), func(v SourceVerdictV2) bool {
				return v.Position == 99
			})

			q.Filter = "position = 99"
			results, _, err := q.Fetch(span.Single(ctx), PageToken{})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, results, should.Match(expected))
		})

		t.Run("With realm filter", func(t *ftt.Test) {
			// Query with "otherrealm" should return only the result in "otherrealm".
			expected := FilterSourceVerdicts(ExpectedSourceVerdicts(), func(v SourceVerdictV2) bool {
				// Verdict 91 is in "otherrealm".
				return v.Position == 91
			})

			q.AllowedSubrealms = []string{"otherrealm"}
			results, _, err := q.Fetch(span.Single(ctx), PageToken{})
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, results, should.Match(expected))
		})
	})
}
