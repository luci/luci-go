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

package buildifier

import (
	"testing"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestNormalizeLintChecks(t *testing.T) {
	t.Parallel()

	diff := func(s1, s2 stringset.Set) []string {
		var d []string
		for _, s := range s1.ToSortedSlice() {
			if !s2.Has(s) {
				d = append(d, s)
			}
		}
		return d
	}

	ftt.Run("Default", t, func(t *ftt.Test) {
		s, err := normalizeLintChecks(nil)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, s.ToSortedSlice(), should.Resemble(defaultChecks().ToSortedSlice()))
	})

	ftt.Run("Add some", t, func(t *ftt.Test) {
		s, err := normalizeLintChecks([]string{
			"+some1",
			"+some2",
			"-some1",
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, diff(s, defaultChecks()), should.Resemble([]string{"some2"}))
	})

	ftt.Run("Remove some", t, func(t *ftt.Test) {
		s, err := normalizeLintChecks([]string{
			"-module-docstring",
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, diff(defaultChecks(), s), should.Resemble([]string{"module-docstring"}))
	})
}
