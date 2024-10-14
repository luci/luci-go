// Copyright 2021 The LUCI Authors.
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

package gerrit

import (
	"fmt"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestFuzzyParseURL(t *testing.T) {
	t.Parallel()

	ftt.Run("FuzzyParseURL works", t, func(t *ftt.Test) {
		h, c, err := FuzzyParseURL("https://crrev.com/i/12/34")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, fmt.Sprintf("%s/%d", h, c), should.Equal("chrome-internal-review.googlesource.com/12"))

		h, c, err = FuzzyParseURL("https://crrev.com/c/12")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, fmt.Sprintf("%s/%d", h, c), should.Equal("chromium-review.googlesource.com/12"))

		h, c, err = FuzzyParseURL("https://chromium-review.googlesource.com/1541677")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, fmt.Sprintf("%s/%d", h, c), should.Equal("chromium-review.googlesource.com/1541677"))

		h, c, err = FuzzyParseURL("https://pdfium-review.googlesource.com/c/33")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, fmt.Sprintf("%s/%d", h, c), should.Equal("pdfium-review.googlesource.com/33"))

		h, c, err = FuzzyParseURL("https://chromium-review.googlesource.com/#/c/infra/luci/luci-go/+/1541677/7")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, fmt.Sprintf("%s/%d", h, c), should.Equal("chromium-review.googlesource.com/1541677"))

		h, c, err = FuzzyParseURL("https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/2652967")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, fmt.Sprintf("%s/%d", h, c), should.Equal("chromium-review.googlesource.com/2652967"))

		h, c, err = FuzzyParseURL("chromium-review.googlesource.com/2652967")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, fmt.Sprintf("%s/%d", h, c), should.Equal("chromium-review.googlesource.com/2652967"))
	})
}
