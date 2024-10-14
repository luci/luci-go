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

package authdb

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/auth/authdb/internal/realmset"
	"go.chromium.org/luci/server/auth/service/protocol"
)

func TestValidation(t *testing.T) {
	t.Parallel()

	validate := func(db *protocol.AuthDB) error {
		_, err := NewSnapshotDB(db, "", 0, true)
		return err
	}

	ftt.Run("Works", t, func(t *ftt.Test) {
		assert.Loosely(t, validate(&protocol.AuthDB{}), should.BeNil)
		assert.Loosely(t, validate(&protocol.AuthDB{
			Groups: []*protocol.AuthGroup{
				{Name: "group"},
			},
			IpWhitelists: []*protocol.AuthIPWhitelist{
				{Name: "IP allowlist"},
			},
		}), should.BeNil)
	})

	ftt.Run("Bad group", t, func(t *ftt.Test) {
		assert.Loosely(t, validate(&protocol.AuthDB{
			Groups: []*protocol.AuthGroup{
				{
					Name:    "group",
					Members: []string{"bad identity"},
				},
			},
		}), should.ErrLike("invalid identity"))
	})

	ftt.Run("Bad IP allowlist", t, func(t *ftt.Test) {
		assert.Loosely(t, validate(&protocol.AuthDB{
			IpWhitelists: []*protocol.AuthIPWhitelist{
				{
					Name:    "IP allowlist",
					Subnets: []string{"not a subnet"},
				},
			},
		}), should.ErrLike("bad IP allowlist"))
	})

	ftt.Run("Bad SecurityConfig", t, func(t *ftt.Test) {
		assert.Loosely(t, validate(&protocol.AuthDB{
			SecurityConfig: []byte("not a serialized proto"),
		}), should.ErrLike("failed to deserialize SecurityConfig"))
	})
}

func TestValidateAuthGroup(t *testing.T) {
	t.Parallel()

	ftt.Run("validateAuthGroup works", t, func(t *ftt.Test) {
		groups := map[string]*protocol.AuthGroup{
			"group1": {
				Name:    "group1",
				Members: []string{"user:abc@example.com"},
				Globs:   []string{"service:*"},
				Nested:  []string{"group2"},
			},
			"group2": {
				Name: "group2",
			},
		}
		assert.Loosely(t, validateAuthGroup("group1", groups), should.BeNil)
	})

	ftt.Run("validateAuthGroup bad identity", t, func(t *ftt.Test) {
		groups := map[string]*protocol.AuthGroup{
			"group1": {
				Name:    "group1",
				Members: []string{"blah"},
			},
		}
		assert.Loosely(t, validateAuthGroup("group1", groups), should.ErrLike("invalid identity"))
	})

	ftt.Run("validateAuthGroup bad glob", t, func(t *ftt.Test) {
		groups := map[string]*protocol.AuthGroup{
			"group1": {
				Name:  "group1",
				Globs: []string{"blah"},
			},
		}
		assert.Loosely(t, validateAuthGroup("group1", groups), should.ErrLike("invalid glob"))
	})

	ftt.Run("validateAuthGroup missing nested group", t, func(t *ftt.Test) {
		groups := map[string]*protocol.AuthGroup{
			"group1": {
				Name:   "group1",
				Nested: []string{"missing"},
			},
		}
		assert.Loosely(t, validateAuthGroup("group1", groups), should.ErrLike("unknown nested group"))
	})

	ftt.Run("validateAuthGroup dependency cycle", t, func(t *ftt.Test) {
		groups := map[string]*protocol.AuthGroup{
			"group1": {
				Name:   "group1",
				Nested: []string{"group1"},
			},
		}
		assert.Loosely(t, validateAuthGroup("group1", groups), should.ErrLike("dependency cycle found"))
	})
}

type groupGraph map[string][]string

func TestFindGroupCycle(t *testing.T) {
	t.Parallel()

	call := func(groups groupGraph) []string {
		asProto := make(map[string]*protocol.AuthGroup)
		for k, v := range groups {
			asProto[k] = &protocol.AuthGroup{
				Name:   k,
				Nested: v,
			}
		}
		return findGroupCycle("start", asProto)
	}

	ftt.Run("Empty", t, func(t *ftt.Test) {
		assert.Loosely(t, call(groupGraph{"start": nil}), should.BeEmpty)
	})

	ftt.Run("No cycles", t, func(t *ftt.Test) {
		assert.Loosely(t, call(groupGraph{
			"start": []string{"A"},
			"A":     []string{"B"},
			"B":     []string{"C"},
		}), should.BeEmpty)
	})

	ftt.Run("Self reference", t, func(t *ftt.Test) {
		assert.Loosely(t, call(groupGraph{
			"start": []string{"start"},
		}), should.Resemble([]string{"start"}))
	})

	ftt.Run("Simple cycle", t, func(t *ftt.Test) {
		assert.Loosely(t, call(groupGraph{
			"start": []string{"A"},
			"A":     []string{"start"},
		}), should.Resemble([]string{"start", "A"}))
	})

	ftt.Run("Long cycle", t, func(t *ftt.Test) {
		assert.Loosely(t, call(groupGraph{
			"start": []string{"A"},
			"A":     []string{"B"},
			"B":     []string{"C"},
			"C":     []string{"start"},
		}), should.Resemble([]string{"start", "A", "B", "C"}))
	})

	ftt.Run("Diamond no cycles", t, func(t *ftt.Test) {
		assert.Loosely(t, call(groupGraph{
			"start": []string{"A1", "A2"},
			"A1":    []string{"B"},
			"A2":    []string{"B"},
			"B":     nil,
		}), should.BeEmpty)
	})

	ftt.Run("Diamond with cycles", t, func(t *ftt.Test) {
		assert.Loosely(t, call(groupGraph{
			"start": []string{"A1", "A2"},
			"A1":    []string{"B"},
			"A2":    []string{"B"},
			"B":     []string{"start"},
		}), should.Resemble([]string{"start", "A1", "B"}))
	})
}

func TestValidateRealms(t *testing.T) {
	t.Parallel()

	validate := func(realms *protocol.Realms) error {
		_, err := NewSnapshotDB(&protocol.AuthDB{Realms: realms}, "", 0, true)
		return err
	}

	perm := &protocol.Permission{}
	cond := &protocol.Condition{}

	ftt.Run("Works", t, func(t *ftt.Test) {
		assert.Loosely(t, validate(&protocol.Realms{
			ApiVersion:  realmset.ExpectedAPIVersion,
			Conditions:  []*protocol.Condition{cond, cond},
			Permissions: []*protocol.Permission{perm, perm},
			Realms: []*protocol.Realm{
				{
					Bindings: []*protocol.Binding{
						{Permissions: []uint32{0}},
						{Conditions: []uint32{0}},
						{Permissions: []uint32{0, 1}, Conditions: []uint32{0, 1}},
					},
				},
			},
		}), should.BeNil)
	})

	ftt.Run("Out of bounds permission", t, func(t *ftt.Test) {
		assert.Loosely(t, validate(&protocol.Realms{
			ApiVersion:  realmset.ExpectedAPIVersion,
			Conditions:  []*protocol.Condition{cond, cond},
			Permissions: []*protocol.Permission{perm, perm},
			Realms: []*protocol.Realm{
				{
					Bindings: []*protocol.Binding{
						{Permissions: []uint32{2}},
					},
				},
			},
		}), should.ErrLike("referencing out-of-bounds permission"))
	})

	ftt.Run("Out of bounds condition", t, func(t *ftt.Test) {
		assert.Loosely(t, validate(&protocol.Realms{
			ApiVersion:  realmset.ExpectedAPIVersion,
			Conditions:  []*protocol.Condition{cond, cond},
			Permissions: []*protocol.Permission{perm, perm},
			Realms: []*protocol.Realm{
				{
					Bindings: []*protocol.Binding{
						{Conditions: []uint32{2}},
					},
				},
			},
		}), should.ErrLike("referencing out-of-bounds condition"))
	})
}
