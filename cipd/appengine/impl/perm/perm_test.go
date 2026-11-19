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

package perm

import (
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestSet(t *testing.T) {
	t.Parallel()

	assert.That(t, DiagnoseACL.Perms(), should.Match(SetOf(DiagnoseACL)))
	assert.That(t, len((Permission{}).Perms()), should.Equal(0))

	var nilCore *corePermission
	assert.That(t, nilCore.Perm().Name(), should.Equal(""))
	assert.That(t, nilCore.String(), should.Equal("<invalid>"))
	assert.That(t, len(nilCore.Perms()), should.Equal(0))

	var s Set
	assert.That(t, s.String(), should.Equal("<empty>"))
	assert.That(t, s.Has(DiagnoseACL), should.BeFalse)
	assert.That(t, s.Perms(), should.Match(Set(nil)))

	s.Add(Permission{})
	assert.That(t, s, should.Match(Set(nil)))

	s.Add(DiagnoseACL)
	assert.That(t, s.Has(DiagnoseACL), should.BeTrue)
	assert.That(t, s.String(), should.Equal("cipd.packages.diagnoseACL"))

	s.Add(testPermWrite)
	s.Add(testPermRead)
	assert.That(t, s.String(), should.Equal("[cipd.packages.diagnoseACL,cipd.testing.read,cipd.testing.write]"))

	var first Permission
	count := 0
	for p := range s.Sorted {
		first = p
		count++
		break
	}
	assert.That(t, count, should.Equal(1))
	assert.That(t, first, should.Equal(DiagnoseACL))
}

func TestBindableUnconditional(t *testing.T) {
	t.Parallel()

	u := testPermAttr.Unconditional()
	assert.That(t, u.Perm(), should.Equal(testPermAttr.Perm()))
	assert.That(t, u.String(), should.Equal("cipd.testing.attr"))
	assert.Loosely(t, u.attrs(), should.BeNil)
	assert.Loosely(t, DiagnoseACL.attrs(), should.BeNil)

	bound := testPermAttr.Bind("v1")
	attrs := bound.attrs()
	assert.That(t, len(attrs), should.Equal(1))
	assert.That(t, attrs["cipd.testing.key"], should.Equal("v1"))

	s := SetOf(testPermAttr.Bind("v2"), u, bound)
	assert.That(t, s.String(), should.Equal(`[cipd.testing.attr,cipd.testing.attr{cipd.testing.key:"v1"},cipd.testing.attr{cipd.testing.key:"v2"}]`))
}

func TestPermission(t *testing.T) {
	t.Parallel()

	assert.That(t, DiagnoseACL.Perm().Name(), should.Equal("cipd.packages.diagnoseACL"))
	assert.That(t, DiagnoseACL.String(), should.Equal("cipd.packages.diagnoseACL"))
	assert.That(t, DiagnoseACL.Group(), should.Equal(PrefixesViewers))
	assert.That(t, DiagnoseACL.IsGroupOnly(), should.BeFalse)
	assert.That(t, DiagnoseACL.IsWrite(), should.BeFalse)
	assert.That(t, (Permission{}).Perm().Name(), should.Equal(""))
	assert.That(t, (Permission{}).String(), should.Equal("<invalid>"))
	assert.That(t, (Bindable{}).Perm().Name(), should.Equal(""))

	t.Run("modifiers_and_binding", func(t *testing.T) {
		t.Parallel()

		assert.That(t, testPermAttr.Perm().Name(), should.Equal("cipd.testing.attr"))

		p1 := testPermAttr.Bind("v1")
		p2 := testPermAttr.Bind("v2")
		assert.That(t, p1.IsWrite(), should.BeFalse)
		assert.That(t, p1.Group(), should.Equal(""))
		assert.That(t, p1.IsGroupOnly(), should.BeFalse)
		assert.That(t, p1.String(), should.Equal(`cipd.testing.attr{cipd.testing.key:"v1"}`))
		assert.That(t, p2.String(), should.Equal(`cipd.testing.attr{cipd.testing.key:"v2"}`))

		assert.That(t, testPermWrite.IsWrite(), should.BeTrue)

		assert.That(t, testPermGroup.IsGroupOnly(), should.BeFalse)
		assert.That(t, testPermGroup.Group(), should.Equal("administrators"))

		assert.That(t, testPermFake.IsWrite(), should.BeTrue)
		assert.That(t, testPermFake.IsGroupOnly(), should.BeTrue)
		assert.That(t, testPermFake.Group(), should.Equal("administrators"))
	})

	t.Run("duplicate_modifiers_panic", func(t *testing.T) {
		t.Parallel()

		assert.That(t, func() {
			testPermWrite.asWrite()
		}, should.Panic)

		assert.That(t, func() {
			DiagnoseACL.withGroup("b")
		}, should.Panic)

		assert.That(t, func() {
			DiagnoseACL.groupOnly("b")
		}, should.Panic)

		assert.That(t, func() {
			testPermFake.groupOnly("b")
		}, should.Panic)

		assert.That(t, func() {
			testPermFake.withGroup("b")
		}, should.Panic)

		assert.That(t, func() {
			testPermAttr.Bind("v").withAttr("k2")
		}, should.Panic)
	})
}
