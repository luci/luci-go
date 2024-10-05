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

package util

import (
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"testing"
)

func TestValidation(t *testing.T) {
	ftt.Run("Validate Project", t, func(t *ftt.Test) {
		assert.Loosely(t, ValidateProject(""), should.NotBeNil)
		// Too long.
		assert.Loosely(t, ValidateProject("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), should.NotBeNil)
		assert.Loosely(t, ValidateProject("chromium"), should.BeNil)
	})

	ftt.Run("Validate Variant Hash", t, func(t *ftt.Test) {
		assert.Loosely(t, ValidateVariantHash("11gg"), should.NotBeNil)
		assert.Loosely(t, ValidateVariantHash("0123456789abcdef"), should.BeNil)
	})

	ftt.Run("Validate Ref Hash", t, func(t *ftt.Test) {
		assert.Loosely(t, ValidateRefHash("11gg"), should.NotBeNil)
		assert.Loosely(t, ValidateRefHash("0123456789abcdef"), should.BeNil)
	})
}
