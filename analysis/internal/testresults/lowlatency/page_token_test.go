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
)

func TestPageToken(t *testing.T) {
	ftt.Run("PageToken", t, func(t *ftt.Test) {
		t.Run("Roundtrip", func(t *ftt.Test) {
			original := PageToken{
				LastSourcePosition: 12345,
			}
			serialized := original.Serialize()
			assert.Loosely(t, serialized, should.NotBeEmpty)

			parsed, err := ParsePageToken(serialized)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, parsed, should.Match(original))
		})

		t.Run("Empty", func(t *ftt.Test) {
			original := PageToken{}
			serialized := original.Serialize()
			assert.Loosely(t, serialized, should.Equal(""))

			parsed, err := ParsePageToken(serialized)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, parsed, should.Match(original))
		})

		t.Run("Invalid", func(t *ftt.Test) {
			_, err := ParsePageToken("invalid-token")
			assert.Loosely(t, err, should.NotBeNil)
		})
	})
}
