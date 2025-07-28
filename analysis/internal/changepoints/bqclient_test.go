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

package changepoints

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/analysis/internal/pagination"
)

func TestParseReadChangepointGroupSummariesPageToken(t *testing.T) {
	ftt.Run(`Test parseReadChangepointGroupSummariesPageToken`, t, func(t *ftt.Test) {
		t.Run(`valid token`, func(t *ftt.Test) {
			nextPageToken := pagination.Token("123", "testid", "variantHash", "refhash", "100")

			afterStartHourUnix, afterTestID, afterVariantHash, afterRefHash, afterNominalStartPosition, err := parseReadChangepointGroupSummariesPageToken(nextPageToken)
			assert.NoErr(t, err)
			assert.That(t, afterStartHourUnix, should.Equal(123))
			assert.That(t, afterTestID, should.Equal("testid"))
			assert.That(t, afterVariantHash, should.Equal("variantHash"))
			assert.That(t, afterRefHash, should.Equal("refhash"))
			assert.That(t, afterNominalStartPosition, should.Equal(100))
		})
		t.Run(`malformated token`, func(t *ftt.Test) {
			nextPageToken := pagination.Token("one", "testid", "variantHash", "refhash", "100")

			_, _, _, _, _, err := parseReadChangepointGroupSummariesPageToken(nextPageToken)
			assert.That(t, err, should.ErrLike("expect the first page_token component to be an integer"))
		})
	})
}
