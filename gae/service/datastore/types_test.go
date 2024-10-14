// Copyright 2015 The LUCI Authors.
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

package datastore

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestExistsResult(t *testing.T) {
	t.Parallel()

	ftt.Run(`Testing ExistsResult`, t, func(t *ftt.Test) {
		var er ExistsResult

		t.Run(`With no elements`, func(t *ftt.Test) {
			er.init()

			assert.Loosely(t, er.All(), should.BeTrue)
			assert.Loosely(t, er.Any(), should.BeFalse)
			assert.Loosely(t, er.List(), should.Resemble(BoolList{}))
		})

		t.Run(`With single-tier elements`, func(t *ftt.Test) {
			er.init(1, 1, 1, 1)

			assert.Loosely(t, er.All(), should.BeFalse)
			assert.Loosely(t, er.Any(), should.BeFalse)
			assert.Loosely(t, er.List(), should.Resemble(BoolList{false, false, false, false}))

			er.set(0, 0)
			er.set(2, 0)

			assert.Loosely(t, er.All(), should.BeFalse)
			assert.Loosely(t, er.Any(), should.BeTrue)
			assert.Loosely(t, er.List(), should.Resemble(BoolList{true, false, true, false}))
			assert.Loosely(t, er.List(0), should.Resemble(BoolList{true}))
			assert.Loosely(t, er.Get(0, 0), should.BeTrue)
			assert.Loosely(t, er.List(1), should.Resemble(BoolList{false}))
			assert.Loosely(t, er.Get(1, 0), should.BeFalse)
			assert.Loosely(t, er.List(2), should.Resemble(BoolList{true}))
			assert.Loosely(t, er.Get(2, 0), should.BeTrue)
			assert.Loosely(t, er.List(3), should.Resemble(BoolList{false}))
			assert.Loosely(t, er.Get(3, 0), should.BeFalse)
		})

		t.Run(`With combined single- and multi-tier elements`, func(t *ftt.Test) {
			er.init(1, 0, 3, 2)

			assert.Loosely(t, er.All(), should.BeFalse)
			assert.Loosely(t, er.Any(), should.BeFalse)
			assert.Loosely(t, er.List(), should.Resemble(BoolList{false, false, false, false}))

			// Set everything except (2, 1).
			er.set(0, 0)
			er.set(2, 0)
			er.set(2, 2)
			er.set(3, 0)
			er.set(3, 1)

			er.updateSlices()
			assert.Loosely(t, er.All(), should.BeFalse)
			assert.Loosely(t, er.Any(), should.BeTrue)
			assert.Loosely(t, er.List(), should.Resemble(BoolList{true, true, false, true}))
			assert.Loosely(t, er.List(0), should.Resemble(BoolList{true}))
			assert.Loosely(t, er.List(1), should.Resemble(BoolList(nil)))
			assert.Loosely(t, er.List(2), should.Resemble(BoolList{true, false, true}))
			assert.Loosely(t, er.Get(2, 0), should.BeTrue)
			assert.Loosely(t, er.Get(2, 1), should.BeFalse)
			assert.Loosely(t, er.List(3), should.Resemble(BoolList{true, true}))

			// Set the missing boolean.
			er.set(2, 1)
			er.updateSlices()
			assert.Loosely(t, er.All(), should.BeTrue)
		})

		t.Run(`Zero-length slices are handled properly.`, func(t *ftt.Test) {
			er.init(1, 0, 0, 1)

			er.updateSlices()
			assert.Loosely(t, er.List(), should.Resemble(BoolList{false, true, true, false}))
			assert.Loosely(t, er.All(), should.BeFalse)
			assert.Loosely(t, er.Any(), should.BeFalse)

			er.set(0, 0)
			er.updateSlices()
			assert.Loosely(t, er.List(), should.Resemble(BoolList{true, true, true, false}))
			assert.Loosely(t, er.All(), should.BeFalse)
			assert.Loosely(t, er.Any(), should.BeTrue)

			er.set(3, 0)
			er.updateSlices()
			assert.Loosely(t, er.List(), should.Resemble(BoolList{true, true, true, true}))
			assert.Loosely(t, er.All(), should.BeTrue)
			assert.Loosely(t, er.Any(), should.BeTrue)
		})
	})
}
