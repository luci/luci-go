// Copyright 2020 The LUCI Authors.
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

package protoutil

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestParentStepName(t *testing.T) {
	t.Parallel()

	ftt.Run("ParentStepName", t, func(t *ftt.Test) {
		t.Run("with an empty name", func(t *ftt.Test) {
			assert.Loosely(t, ParentStepName(""), should.BeEmpty)
		})

		t.Run("with a parent", func(t *ftt.Test) {
			assert.Loosely(t, ParentStepName("a|b"), should.Equal("a"))
			assert.Loosely(t, ParentStepName("a|b|c"), should.Equal("a|b"))
			assert.Loosely(t, ParentStepName("|b|"), should.Equal("|b"))
		})

		t.Run("without a paraent", func(t *ftt.Test) {
			assert.Loosely(t, ParentStepName("a.b"), should.BeEmpty)
		})
	})
}

func TestValidateStepName(t *testing.T) {
	t.Parallel()

	ftt.Run("Validate", t, func(t *ftt.Test) {
		t.Run("with an empty name", func(t *ftt.Test) {
			assert.Loosely(t, ValidateStepName(""), should.ErrLike("required"))
		})

		t.Run("with valid names", func(t *ftt.Test) {
			assert.Loosely(t, ValidateStepName("a"), should.BeNil)
			assert.Loosely(t, ValidateStepName("a|b"), should.BeNil)
			assert.Loosely(t, ValidateStepName("a|b|c"), should.BeNil)
		})

		t.Run("with invalid names", func(t *ftt.Test) {
			errMsg := `there must be at least one character before and after "|"`
			assert.Loosely(t, ValidateStepName("|"), should.ErrLike(errMsg))
			assert.Loosely(t, ValidateStepName("a|"), should.ErrLike(errMsg))
			assert.Loosely(t, ValidateStepName("|a"), should.ErrLike(errMsg))
			assert.Loosely(t, ValidateStepName("a||b"), should.ErrLike(errMsg))
		})
	})
}
