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

package test_outputs

import (
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"testing"
)

func TestResemble(t *testing.T) {
	t.Parallel()

	ftt.Run("something", t, func(t *ftt.Test) {
		assert.Loosely(t, 0, should.BeZero)
		assert.Loosely(t, 100, should.Match(100))

		assert.Loosely(t, "", should.BeBlank)

		assert.Loosely(t, nil, should.BeNil)

		assert.Loosely(t, "nerb", should.Match("nerb"))

		type myType struct{ f int }
		assert.Loosely(t, myType{100}, should.Resemble(myType{100}))
	})
}

func TestNotResemble(t *testing.T) {
	t.Parallel()

	ftt.Run("something", t, func(t *ftt.Test) {
		assert.Loosely(t, 1, should.NotBeZero)
		assert.Loosely(t, 101, should.NotMatch(100))

		assert.Loosely(t, "1", should.NotBeBlank)

		assert.Loosely(t, &(struct{}{}), should.NotBeNil)

		assert.Loosely(t, "nerb", should.NotMatch("nerb1"))

		type myType struct{ f int }
		assert.Loosely(t, myType{101}, should.NotResemble(myType{100}))
	})
}
