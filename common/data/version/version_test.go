// Copyright 2026 The LUCI Authors.
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

package version

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestVersion(t *testing.T) {
	ftt.Run("Parse", t, func(t *ftt.Test) {
		t.Run("1 component", func(t *ftt.Test) {
			v, err := Parse("1")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, v, should.Equal(Version{Major: 1}))
			assert.Loosely(t, v.String(), should.Equal("1.0.0.0"))
		})

		t.Run("2 components", func(t *ftt.Test) {
			v, err := Parse("1.2")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, v, should.Equal(Version{Major: 1, Minor: 2}))
			assert.Loosely(t, v.String(), should.Equal("1.2.0.0"))
		})

		t.Run("3 components", func(t *ftt.Test) {
			v, err := Parse("1.2.3")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, v, should.Equal(Version{
				Major: 1, Minor: 2, Patch: 3,
			}))
			assert.Loosely(t, v.String(), should.Equal("1.2.3.0"))
		})

		t.Run("4 components", func(t *ftt.Test) {
			v, err := Parse("150.0.7000.42")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, v, should.Equal(Version{
				Major: 150, Minor: 0, Patch: 7000, Build: 42,
			}))
			assert.Loosely(t, v.String(), should.Equal("150.0.7000.42"))
		})

		t.Run("Invalid formats", func(t *ftt.Test) {
			for _, tc := range []string{
				"",
				"v",
				"v1.2.3",
				" 1.2.3 ",
				"1.2.3.4.5",
				"1..2",
				"1.2.foo",
				"-1",
				"1.2.3.-4",
			} {
				_, err := Parse(tc)
				assert.Loosely(t, err, should.NotBeNil)
			}
		})

		t.Run("Component overflow", func(t *ftt.Test) {
			_, err := Parse("999999999999999999999999999999.0.0")
			assert.Loosely(t, err, should.NotBeNil)

			_, err = Parse("1.0.0.999999999999999999999999999999")
			assert.Loosely(t, err, should.NotBeNil)
		})
	})

	ftt.Run("MustParse", t, func(t *ftt.Test) {
		t.Run("Valid", func(t *ftt.Test) {
			v := MustParse("1.2.3.4")
			assert.Loosely(t, v.Build, should.Equal(uint64(4)))
		})

		t.Run("Panics on invalid", func(t *ftt.Test) {
			assert.Loosely(t, func() { MustParse("invalid") }, should.Panic)
		})
	})

	ftt.Run("Compare and Less", t, func(t *ftt.Test) {
		v100 := MustParse("150.0.7000.0")
		v101 := MustParse("151.0.7001.0")
		v102 := MustParse("150.0.7000.1")
		v103 := MustParse("151.0.7002.0")

		t.Run("Equal", func(t *ftt.Test) {
			assert.Loosely(t, v100.Compare(v100), should.BeZero)
			assert.Loosely(t, v100.Less(v100), should.BeFalse)
		})

		t.Run("Major difference", func(t *ftt.Test) {
			assert.Loosely(t, v100.Compare(v101), should.Equal(-1))
			assert.Loosely(t, v101.Compare(v100), should.Equal(1))
			assert.Loosely(t, v100.Less(v101), should.BeTrue)
			assert.Loosely(t, v101.Less(v100), should.BeFalse)
		})

		t.Run("Minor difference", func(t *ftt.Test) {
			va := MustParse("150.1.7000.0")
			assert.Loosely(t, v100.Compare(va), should.Equal(-1))
			assert.Loosely(t, va.Compare(v100), should.Equal(1))
			assert.Loosely(t, v100.Less(va), should.BeTrue)
		})

		t.Run("Patch difference", func(t *ftt.Test) {
			va := MustParse("150.0.7001.0")
			assert.Loosely(t, v100.Compare(va), should.Equal(-1))
			assert.Loosely(t, va.Compare(v100), should.Equal(1))
			assert.Loosely(t, v100.Less(va), should.BeTrue)
		})

		t.Run("Build difference", func(t *ftt.Test) {
			assert.Loosely(t, v100.Compare(v102), should.Equal(-1))
			assert.Loosely(t, v102.Compare(v100), should.Equal(1))
			assert.Loosely(t, v100.Less(v102), should.BeTrue)
		})

		t.Run("Ordering across versions", func(t *ftt.Test) {
			assert.Loosely(t, v100.Less(v102), should.BeTrue)
			assert.Loosely(t, v102.Less(v101), should.BeTrue)
			assert.Loosely(t, v101.Less(v103), should.BeTrue)
		})

		t.Run("Shorter component versions", func(t *ftt.Test) {
			v1 := MustParse("1")
			v1Zero := MustParse("1.0.0.0")
			v1Dot1 := MustParse("1.1")

			assert.Loosely(t, v1.Compare(v1Zero), should.BeZero)
			assert.Loosely(t, v1.Less(v1Dot1), should.BeTrue)
			assert.Loosely(t, v1Dot1.Less(v1), should.BeFalse)
		})
	})
}
