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

package pbutil

import (
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"testing"
)

func TestTestExonerationName(t *testing.T) {
	t.Parallel()
	ftt.Run("ParseTestExonerationName", t, func(t *ftt.Test) {
		t.Run("Parse", func(t *ftt.Test) {
			inv, testID, ex, err := ParseTestExonerationName("invocations/a/tests/b%2Fc/exonerations/1")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, inv, should.Equal("a"))
			assert.Loosely(t, testID, should.Equal("b/c"))
			assert.Loosely(t, ex, should.Equal("1"))
		})

		t.Run("Invalid", func(t *ftt.Test) {
			_, _, _, err := ParseTestExonerationName("invocations/a/tests/b/c/exonerations/1")
			assert.Loosely(t, err, should.ErrLike(`does not match`))
		})

		t.Run("Format", func(t *ftt.Test) {
			assert.Loosely(t, TestExonerationName("a", "b/c", "d"), should.Equal("invocations/a/tests/b%2Fc/exonerations/d"))
		})
	})
}
