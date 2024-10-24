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

package realms

import (
	"fmt"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestValidatePermissionName(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
		assert.Loosely(t, ValidatePermissionName("service.subject.verb"), should.BeNil)

		assert.Loosely(t, ValidatePermissionName("service.subject.verb.stuff"), should.NotBeNil)
		assert.Loosely(t, ValidatePermissionName("service.subject"), should.NotBeNil)
		assert.Loosely(t, ValidatePermissionName("service.subject."), should.NotBeNil)
		assert.Loosely(t, ValidatePermissionName("service..verb"), should.NotBeNil)
		assert.Loosely(t, ValidatePermissionName(".subject.verb"), should.NotBeNil)
		assert.Loosely(t, ValidatePermissionName(""), should.NotBeNil)
	})
}

func TestRegister(t *testing.T) {
	// This test interacts with the global `perms` cache.
	// t.Parallel()

	ftt.Run("TestRegister", t, func(t *ftt.Test) {
		// Make sure the test succeeds when using `go test . -count=2`.
		clearPermissions()

		t.Run("Works", func(t *ftt.Test) {
			p1 := RegisterPermission("luci.dev.testing1")
			assert.Loosely(t, p1.Name(), should.Equal("luci.dev.testing1"))
			assert.Loosely(t, p1.String(), should.Equal("luci.dev.testing1"))
			assert.Loosely(t, fmt.Sprintf("%q", p1), should.Equal(`"luci.dev.testing1"`))

			p2 := RegisterPermission("luci.dev.testing2")
			p2.AddFlags(UsedInQueryRealms)

			// Reregistering doesn't clear the flags.
			RegisterPermission("luci.dev.testing2")
			assert.Loosely(t, RegisteredPermissions(), should.Resemble(map[Permission]PermissionFlags{
				p1: 0,
				p2: UsedInQueryRealms,
			}))
		})

		t.Run("Panics on bad name", func(t *ftt.Test) {
			assert.Loosely(t, func() { RegisterPermission(".bad.name") }, should.Panic)
		})

		t.Run("Panics on mutation after freeze", func(t *ftt.Test) {
			p1 := RegisterPermission("luci.dev.testing1")
			_ = RegisteredPermissions()
			assert.Loosely(t, func() { RegisterPermission("luci.dev.testing1") }, should.Panic)
			assert.Loosely(t, func() { p1.AddFlags(UsedInQueryRealms) }, should.Panic)
		})
	})
}

func TestGetPermissions(t *testing.T) {
	// This test interacts with the global `perms` cache.
	// t.Parallel()

	ftt.Run("TestGetPermissions", t, func(t *ftt.Test) {
		clearPermissions()
		RegisterPermission("luci.dev.testing1")
		RegisterPermission("luci.dev.testing2")

		t.Run("Get single permission works", func(t *ftt.Test) {
			perms, err := GetPermissions("luci.dev.testing1")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, perms, should.HaveLength(1))
			assert.Loosely(t, perms[0].Name(), should.Equal("luci.dev.testing1"))
		})

		t.Run("Get multiple permissions works", func(t *ftt.Test) {
			perms, err := GetPermissions("luci.dev.testing1", "luci.dev.testing2")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, perms, should.HaveLength(2))
			assert.Loosely(t, perms[0].Name(), should.Equal("luci.dev.testing1"))
			assert.Loosely(t, perms[1].Name(), should.Equal("luci.dev.testing2"))

			// Get in a different order.
			perms, err = GetPermissions("luci.dev.testing2", "luci.dev.testing1")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, perms, should.HaveLength(2))
			assert.Loosely(t, perms[0].Name(), should.Equal("luci.dev.testing2"))
			assert.Loosely(t, perms[1].Name(), should.Equal("luci.dev.testing1"))

			// Get duplicates.
			perms, err = GetPermissions("luci.dev.testing1", "luci.dev.testing1")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, perms, should.HaveLength(2))
			assert.Loosely(t, perms[0].Name(), should.Equal("luci.dev.testing1"))
			assert.Loosely(t, perms[1].Name(), should.Equal("luci.dev.testing1"))
		})

		t.Run("Get unregistered permission returns error", func(t *ftt.Test) {
			perms, err := GetPermissions("luci.dev.unregistered")
			assert.Loosely(t, err, should.ErrLike("permission not registered"))
			assert.Loosely(t, err, should.ErrLike("luci.dev.unregistered"))
			assert.Loosely(t, perms, should.BeNil)

			// Mixed with registered permission.
			perms, err = GetPermissions("luci.dev.testing1", "luci.dev.unregistered")
			assert.Loosely(t, err, should.ErrLike("permission not registered"))
			assert.Loosely(t, err, should.ErrLike("luci.dev.unregistered"))
			assert.Loosely(t, perms, should.BeNil)
		})
	})
}
