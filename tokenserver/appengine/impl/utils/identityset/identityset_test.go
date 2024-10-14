// Copyright 2016 The LUCI Authors.
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

package identityset

import (
	"context"
	"testing"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
)

func TestFromStrings(t *testing.T) {
	ftt.Run("Empty", t, func(t *ftt.Test) {
		s, err := FromStrings(nil, nil)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, s, should.Resemble(&Set{}))
		assert.Loosely(t, s.IsEmpty(), should.BeTrue)
		assert.Loosely(t, s.ToStrings(), should.BeEmpty)
	})

	ftt.Run("Universal", t, func(t *ftt.Test) {
		s, err := FromStrings([]string{"*", "user:abc@example.com"}, nil)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, s, should.Resemble(&Set{All: true}))
		assert.Loosely(t, s.IsEmpty(), should.BeFalse)
		assert.Loosely(t, s.ToStrings(), should.Resemble([]string{"*"}))
	})

	ftt.Run("Normal", t, func(t *ftt.Test) {
		s, err := FromStrings([]string{
			"user:abc@example.com",
			"user:def@example.com",
			"user:abc@example.com",
			"skipped",
			"group:abc",
			"group:def",
			"group:abc",
		}, func(s string) bool { return s == "skipped" })
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, s, should.Resemble(&Set{
			IDs: identSet{
				"user:abc@example.com": struct{}{},
				"user:def@example.com": struct{}{},
			},
			Groups: groupSet{
				"abc": struct{}{},
				"def": struct{}{},
			},
		}))
		assert.Loosely(t, s.IsEmpty(), should.BeFalse)
		assert.Loosely(t, s.ToStrings(), should.Resemble([]string{
			"group:abc",
			"group:def",
			"user:abc@example.com",
			"user:def@example.com",
		}))
	})

	ftt.Run("Bad group entry", t, func(t *ftt.Test) {
		s, err := FromStrings([]string{"group:"}, nil)
		assert.Loosely(t, err, should.ErrLike("invalid entry"))
		assert.Loosely(t, s, should.BeNil)
	})

	ftt.Run("Bad ID entry", t, func(t *ftt.Test) {
		s, err := FromStrings([]string{"shrug"}, nil)
		assert.Loosely(t, err, should.ErrLike("bad identity string"))
		assert.Loosely(t, s, should.BeNil)
	})
}

func TestIsMember(t *testing.T) {
	c := context.Background()
	c = auth.WithState(c, &authtest.FakeState{
		Identity:       "user:abc@example.com",
		IdentityGroups: []string{"abc"},
	})

	ftt.Run("nil", t, func(t *ftt.Test) {
		var s *Set
		ok, err := s.IsMember(c, identity.AnonymousIdentity)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, ok, should.BeFalse)
	})

	ftt.Run("All", t, func(t *ftt.Test) {
		s := Set{All: true}
		ok, err := s.IsMember(c, identity.AnonymousIdentity)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, ok, should.BeTrue)
	})

	ftt.Run("Direct hit", t, func(t *ftt.Test) {
		s, _ := FromStrings([]string{"user:abc@example.com"}, nil)

		ok, err := s.IsMember(c, identity.Identity("user:abc@example.com"))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, ok, should.BeTrue)

		ok, err = s.IsMember(c, identity.AnonymousIdentity)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, ok, should.BeFalse)
	})

	ftt.Run("Groups hit", t, func(t *ftt.Test) {
		s, _ := FromStrings([]string{"group:abc"}, nil)

		ok, err := s.IsMember(c, identity.Identity("user:abc@example.com"))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, ok, should.BeTrue)

		ok, err = s.IsMember(c, identity.AnonymousIdentity)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, ok, should.BeFalse)
	})
}

func TestIsSubset(t *testing.T) {
	empty := &Set{}
	all := &Set{All: true}
	some, _ := FromStrings([]string{
		"user:abc@example.com",
		"user:def@example.com",
		"group:abc",
		"group:def",
	}, nil)
	some1, _ := FromStrings([]string{
		"user:abc@example.com",
		"user:def@example.com",
	}, nil)
	some2, _ := FromStrings([]string{
		"group:abc",
		"group:def",
	}, nil)
	some3, _ := FromStrings([]string{
		"user:abc@example.com",
		"group:abc",
	}, nil)
	some4, _ := FromStrings([]string{
		"user:xxx@example.com",
		"user:yyy@example.com",
		"group:xxx",
		"group:yyy",
	}, nil)

	ftt.Run("empty", t, func(t *ftt.Test) {
		assert.Loosely(t, empty.IsSubset(empty), should.BeTrue)
		assert.Loosely(t, empty.IsSubset(all), should.BeTrue)
		assert.Loosely(t, empty.IsSubset(some), should.BeTrue)

		assert.Loosely(t, empty.IsSuperset(empty), should.BeTrue) // for code coverage
	})

	ftt.Run("all", t, func(t *ftt.Test) {
		assert.Loosely(t, all.IsSubset(empty), should.BeFalse)
		assert.Loosely(t, all.IsSubset(all), should.BeTrue)
		assert.Loosely(t, all.IsSubset(some), should.BeFalse)
	})

	ftt.Run("some", t, func(t *ftt.Test) {
		assert.Loosely(t, some.IsSubset(empty), should.BeFalse)
		assert.Loosely(t, some.IsSubset(all), should.BeTrue)
		assert.Loosely(t, some.IsSubset(some), should.BeTrue)

		assert.Loosely(t, some1.IsSubset(some), should.BeTrue)
		assert.Loosely(t, some2.IsSubset(some), should.BeTrue)
		assert.Loosely(t, some3.IsSubset(some), should.BeTrue)

		assert.Loosely(t, some1.IsSubset(some2), should.BeFalse)
		assert.Loosely(t, some1.IsSubset(some3), should.BeFalse)
		assert.Loosely(t, some2.IsSubset(some1), should.BeFalse)
		assert.Loosely(t, some3.IsSubset(some1), should.BeFalse)

		assert.Loosely(t, some1.IsSubset(some3), should.BeFalse)
		assert.Loosely(t, some1.IsSubset(some4), should.BeFalse)

		assert.Loosely(t, some2.IsSubset(some3), should.BeFalse)
		assert.Loosely(t, some2.IsSubset(some4), should.BeFalse)
	})
}

func TestUnion(t *testing.T) {
	empty := &Set{}
	all := &Set{All: true}
	some, _ := FromStrings([]string{
		"user:abc@example.com",
		"user:def@example.com",
		"group:abc",
		"group:def",
	}, nil)
	some1, _ := FromStrings([]string{
		"user:abc@example.com",
		"user:def@example.com",
	}, nil)
	some2, _ := FromStrings([]string{
		"group:abc",
		"group:def",
	}, nil)
	some3, _ := FromStrings([]string{
		"user:abc@example.com",
		"group:abc",
	}, nil)

	ftt.Run("empty", t, func(t *ftt.Test) {
		assert.Loosely(t, Union(), should.Resemble(empty))
		assert.Loosely(t, Union(empty, nil), should.Resemble(empty))
	})

	ftt.Run("one", t, func(t *ftt.Test) {
		assert.Loosely(t, Union(some), should.Resemble(some))
		assert.Loosely(t, Union(some, empty), should.Resemble(some))
	})

	ftt.Run("many", t, func(t *ftt.Test) {
		assert.Loosely(t, Union(some1, some2, some3, empty), should.Resemble(some))
	})

	ftt.Run("all", t, func(t *ftt.Test) {
		assert.Loosely(t, Union(all), should.Resemble(all))
		assert.Loosely(t, Union(all, empty), should.Resemble(all))
		assert.Loosely(t, Union(all, some1), should.Resemble(all))
	})
}

func TestExtend(t *testing.T) {
	ftt.Run("Empty", t, func(t *ftt.Test) {
		assert.Loosely(t, Extend(nil, "user:abc@example.com"), should.Resemble(&Set{
			IDs: identSet{"user:abc@example.com": struct{}{}},
		}))
	})

	ftt.Run("All", t, func(t *ftt.Test) {
		all := &Set{All: true}
		assert.Loosely(t, Extend(all, "user:abc@example.com"), should.Resemble(all))
	})

	ftt.Run("Already there", t, func(t *ftt.Test) {
		set, _ := FromStrings([]string{
			"user:abc@example.com",
			"group:abc",
		}, nil)
		assert.Loosely(t, Extend(set, "user:abc@example.com"), should.Resemble(set))
	})

	ftt.Run("Extends", t, func(t *ftt.Test) {
		set, _ := FromStrings([]string{
			"user:def@example.com",
			"group:abc",
		}, nil)
		assert.Loosely(t, Extend(set, "user:abc@example.com"), should.Resemble(&Set{
			IDs: identSet{
				"user:abc@example.com": struct{}{},
				"user:def@example.com": struct{}{},
			},
			Groups: groupSet{"abc": struct{}{}},
		}))
	})
}
