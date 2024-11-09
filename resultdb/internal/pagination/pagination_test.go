// Copyright 2019 The LUCI Authors.
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

package pagination

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestPageToken(t *testing.T) {
	t.Parallel()

	ftt.Run(`Token works`, t, func(t *ftt.Test) {
		assert.Loosely(t, Token("v1", "v2"), should.Match("CgJ2MQoCdjI="))

		pos, err := ParseToken("CgJ2MQoCdjI=")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, pos, should.Resemble([]string{"v1", "v2"}))

		t.Run(`For fresh token`, func(t *ftt.Test) {
			assert.Loosely(t, Token(), should.BeBlank)

			pos, err := ParseToken("")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pos, should.BeNil)
		})
	})
}

func TestAdjustPageSize(t *testing.T) {
	t.Parallel()
	ftt.Run(`AdjustPageSize`, t, func(t *ftt.Test) {
		t.Run(`OK`, func(t *ftt.Test) {
			assert.Loosely(t, AdjustPageSize(50), should.Equal(50))
		})
		t.Run(`Too big`, func(t *ftt.Test) {
			assert.Loosely(t, AdjustPageSize(1e6), should.Equal(pageSizeMax))
		})
		t.Run(`Missing or 0`, func(t *ftt.Test) {
			assert.Loosely(t, AdjustPageSize(0), should.Equal(pageSizeDefault))
		})
	})
}
