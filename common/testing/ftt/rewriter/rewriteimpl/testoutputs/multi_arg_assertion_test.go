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

package testoutputs

import (
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"testing"
)

func TestMultiArgAssertion(t *testing.T) {
	t.Parallel()

	ftt.Run("something", t, func(t *ftt.Test) {
		s := "hello"
		assert.Loosely(t, s, should.BeIn("hello", "there"))
		assert.Loosely(t, s, should.BeIn([]string{"hello", "there"}...))

		assert.Loosely(t, "morp", should.NotBeIn("hello", "there"))
		assert.Loosely(t, "morp", should.NotBeIn([]string{"hello", "there"}...))
	})
}
