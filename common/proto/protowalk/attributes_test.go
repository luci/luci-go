// Copyright 2022 The LUCI Authors.
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

package protowalk

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestRecurseAttr(t *testing.T) {
	t.Parallel()

	ftt.Run(`RecurseAttr`, t, func(t *ftt.Test) {
		t.Run(`isMap`, func(t *ftt.Test) {
			assert.Loosely(t, recurseNone.isMap(), should.BeFalse)
			assert.Loosely(t, recurseOne.isMap(), should.BeFalse)
			assert.Loosely(t, recurseRepeated.isMap(), should.BeFalse)

			assert.Loosely(t, recurseMapKind.isMap(), should.BeTrue)
			assert.Loosely(t, recurseMapBool.isMap(), should.BeTrue)
			assert.Loosely(t, recurseMapInt.isMap(), should.BeTrue)
			assert.Loosely(t, recurseMapUint.isMap(), should.BeTrue)
			assert.Loosely(t, recurseMapString.isMap(), should.BeTrue)
		})

		t.Run(`set`, func(t *ftt.Test) {
			r := recurseNone

			t.Run(`OK`, func(t *ftt.Test) {
				assert.Loosely(t, r.set(recurseOne), should.BeTrue)
				assert.Loosely(t, r, should.Equal(recurseOne))
				assert.Loosely(t, r.set(recurseOne), should.BeTrue)
				assert.Loosely(t, r, should.Equal(recurseOne))
			})

			t.Run(`bad`, func(t *ftt.Test) {
				assert.Loosely(t, r.set(recurseOne), should.BeTrue)
				assert.Loosely(t, r, should.Equal(recurseOne))
				assert.Loosely(t, func() { r.set(recurseRepeated) }, should.Panic)
			})
		})
	})
}

func TestProcessAttr(t *testing.T) {
	t.Parallel()

	ftt.Run(`ProcessAttr`, t, func(t *ftt.Test) {
		t.Run(`applies`, func(t *ftt.Test) {
			assert.Loosely(t, ProcessNever.applies(false), should.BeFalse)
			assert.Loosely(t, ProcessNever.applies(true), should.BeFalse)

			assert.Loosely(t, ProcessIfSet.applies(false), should.BeFalse)
			assert.Loosely(t, ProcessIfSet.applies(true), should.BeTrue)

			assert.Loosely(t, ProcessIfUnset.applies(false), should.BeTrue)
			assert.Loosely(t, ProcessIfUnset.applies(true), should.BeFalse)

			assert.Loosely(t, ProcessAlways.applies(false), should.BeTrue)
			assert.Loosely(t, ProcessAlways.applies(true), should.BeTrue)
		})

		t.Run(`Valid`, func(t *ftt.Test) {
			assert.Loosely(t, ProcessNever.Valid(), should.BeTrue)
			assert.Loosely(t, ProcessIfSet.Valid(), should.BeTrue)
			assert.Loosely(t, ProcessIfUnset.Valid(), should.BeTrue)
			assert.Loosely(t, ProcessAlways.Valid(), should.BeTrue)

			assert.Loosely(t, ProcessAttr(217).Valid(), should.BeFalse)
		})
	})
}
