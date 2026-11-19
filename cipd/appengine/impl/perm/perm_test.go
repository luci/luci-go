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

func TestPermission(t *testing.T) {
	t.Parallel()

	assert.That(t, DiagnoseACL.Perm().Name(), should.Equal("cipd.packages.diagnoseACL"))
	assert.That(t, DiagnoseACL.String(), should.Equal("cipd.packages.diagnoseACL"))
	assert.That(t, (Permission{}).Perm().Name(), should.Equal(""))
	assert.That(t, (Permission{}).String(), should.Equal("<invalid>"))
}
