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
	"go.chromium.org/luci/server/auth/realms"

	repopb "go.chromium.org/luci/cipd/api/cipd/v1/repopb"
)

func TestRegisteredPermissionsInvariants(t *testing.T) {
	t.Parallel()

	assert.That(t, len(AllPermissions), should.Equal(19))
	seen := make(map[realms.Permission]struct{}, len(AllPermissions))
	for g := range AllPermissions {
		seen[g.Perm()] = struct{}{}

		core, ok := g.(*corePermission)
		if !ok {
			t.Fatalf("unexpected Grantable type %T", g)
		}
		assert.Loosely(t, core, should.NotBeNil)

		if core.isWrite && core.role != repopb.Role_ROLE_UNSPECIFIED {
			assert.Loosely(t, core.role, should.BeGreaterThanOrEqual(repopb.Role_WRITER))
		}
	}
	assert.That(t, len(seen), should.Equal(len(AllPermissions)))
}

func TestAllPermissionsCheckable(t *testing.T) {
	t.Parallel()

	allCheckable := AllPermissionsSet()
	assert.That(t, len(allCheckable), should.Equal(len(AllPermissions)))
	for g := range AllPermissions {
		assert.That(t, g.String(), should.Equal(g.Perm().Name()))
		gPerms := g.Perms()
		assert.That(t, len(gPerms), should.Equal(1))
		for p := range gPerms {
			assert.That(t, allCheckable.Has(p), should.BeTrue)
		}
	}
	assert.That(t, allCheckable.Has(WriteRef.Unconditional()), should.BeTrue)
	assert.That(t, allCheckable.Has(WriteRef.Bind("v1")), should.BeFalse)
}
