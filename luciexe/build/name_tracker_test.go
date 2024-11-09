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

package build

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestNameTracker(t *testing.T) {
	t.Parallel()

	ftt.Run(`Name Tracker`, t, func(t *ftt.Test) {
		nt := &nameTracker{}

		t.Run(`unique names`, func(t *ftt.Test) {
			assert.Loosely(t, nt.resolveName("hello"), should.Equal("hello"))
			assert.Loosely(t, nt.resolveName("bob"), should.Equal("bob"))
		})

		t.Run(`simple duplicates`, func(t *ftt.Test) {
			assert.Loosely(t, nt.resolveName("hello"), should.Equal("hello"))
			assert.Loosely(t, nt.resolveName("hello"), should.Equal("hello (2)"))
			assert.Loosely(t, nt.resolveName("hello"), should.Equal("hello (3)"))
		})

		t.Run(`poisoned duplicates`, func(t *ftt.Test) {
			assert.Loosely(t, nt.resolveName("hello (2)"), should.Equal("hello (2)"))

			assert.Loosely(t, nt.resolveName("hello"), should.Equal("hello"))
			assert.Loosely(t, nt.resolveName("hello"), should.Equal("hello (3)"))
			assert.Loosely(t, nt.resolveName("hello"), should.Equal("hello (4)"))
		})
	})
}
