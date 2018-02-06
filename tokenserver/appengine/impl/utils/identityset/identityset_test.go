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
	"testing"

	"golang.org/x/net/context"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestFromStrings(t *testing.T) {
	Convey("Empty", t, func() {
		s, err := FromStrings(nil, nil)
		So(err, ShouldBeNil)
		So(s, ShouldResemble, &Set{})
		So(s.IsEmpty(), ShouldBeTrue)
		So(s.ToStrings(), ShouldBeEmpty)
	})

	Convey("Universal", t, func() {
		s, err := FromStrings([]string{"*", "user:abc@example.com"}, nil)
		So(err, ShouldBeNil)
		So(s, ShouldResemble, &Set{All: true})
		So(s.IsEmpty(), ShouldBeFalse)
		So(s.ToStrings(), ShouldResemble, []string{"*"})
	})

	Convey("Normal", t, func() {
		s, err := FromStrings([]string{
			"user:abc@example.com",
			"user:def@example.com",
			"user:abc@example.com",
			"skipped",
			"group:abc",
			"group:def",
			"group:abc",
		}, func(s string) bool { return s == "skipped" })
		So(err, ShouldBeNil)
		So(s, ShouldResemble, &Set{
			IDs: identSet{
				"user:abc@example.com": struct{}{},
				"user:def@example.com": struct{}{},
			},
			Groups: groupSet{
				"abc": struct{}{},
				"def": struct{}{},
			},
		})
		So(s.IsEmpty(), ShouldBeFalse)
		So(s.ToStrings(), ShouldResemble, []string{
			"group:abc",
			"group:def",
			"user:abc@example.com",
			"user:def@example.com",
		})
	})

	Convey("Bad group entry", t, func() {
		s, err := FromStrings([]string{"group:"}, nil)
		So(err, ShouldErrLike, "invalid entry")
		So(s, ShouldEqual, nil)
	})

	Convey("Bad ID entry", t, func() {
		s, err := FromStrings([]string{"shrug"}, nil)
		So(err, ShouldErrLike, "bad identity string")
		So(s, ShouldEqual, nil)
	})
}

func TestIsMember(t *testing.T) {
	c := context.Background()
	c = auth.WithState(c, &authtest.FakeState{
		Identity:       "user:abc@example.com",
		IdentityGroups: []string{"abc"},
	})

	Convey("nil", t, func() {
		var s *Set
		ok, err := s.IsMember(c, identity.AnonymousIdentity)
		So(err, ShouldBeNil)
		So(ok, ShouldBeFalse)
	})

	Convey("All", t, func() {
		s := Set{All: true}
		ok, err := s.IsMember(c, identity.AnonymousIdentity)
		So(err, ShouldBeNil)
		So(ok, ShouldBeTrue)
	})

	Convey("Direct hit", t, func() {
		s, _ := FromStrings([]string{"user:abc@example.com"}, nil)

		ok, err := s.IsMember(c, identity.Identity("user:abc@example.com"))
		So(err, ShouldBeNil)
		So(ok, ShouldBeTrue)

		ok, err = s.IsMember(c, identity.AnonymousIdentity)
		So(err, ShouldBeNil)
		So(ok, ShouldBeFalse)
	})

	Convey("Groups hit", t, func() {
		s, _ := FromStrings([]string{"group:abc"}, nil)

		ok, err := s.IsMember(c, identity.Identity("user:abc@example.com"))
		So(err, ShouldBeNil)
		So(ok, ShouldBeTrue)

		ok, err = s.IsMember(c, identity.AnonymousIdentity)
		So(err, ShouldBeNil)
		So(ok, ShouldBeFalse)
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

	Convey("empty", t, func() {
		So(empty.IsSubset(empty), ShouldBeTrue)
		So(empty.IsSubset(all), ShouldBeTrue)
		So(empty.IsSubset(some), ShouldBeTrue)

		So(empty.IsSuperset(empty), ShouldBeTrue) // for code coverage
	})

	Convey("all", t, func() {
		So(all.IsSubset(empty), ShouldBeFalse)
		So(all.IsSubset(all), ShouldBeTrue)
		So(all.IsSubset(some), ShouldBeFalse)
	})

	Convey("some", t, func() {
		So(some.IsSubset(empty), ShouldBeFalse)
		So(some.IsSubset(all), ShouldBeTrue)
		So(some.IsSubset(some), ShouldBeTrue)

		So(some1.IsSubset(some), ShouldBeTrue)
		So(some2.IsSubset(some), ShouldBeTrue)
		So(some3.IsSubset(some), ShouldBeTrue)

		So(some1.IsSubset(some2), ShouldBeFalse)
		So(some1.IsSubset(some3), ShouldBeFalse)
		So(some2.IsSubset(some1), ShouldBeFalse)
		So(some3.IsSubset(some1), ShouldBeFalse)

		So(some1.IsSubset(some3), ShouldBeFalse)
		So(some1.IsSubset(some4), ShouldBeFalse)

		So(some2.IsSubset(some3), ShouldBeFalse)
		So(some2.IsSubset(some4), ShouldBeFalse)
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

	Convey("empty", t, func() {
		So(Union(), ShouldResemble, empty)
		So(Union(empty, nil), ShouldResemble, empty)
	})

	Convey("one", t, func() {
		So(Union(some), ShouldResemble, some)
		So(Union(some, empty), ShouldResemble, some)
	})

	Convey("many", t, func() {
		So(Union(some1, some2, some3, empty), ShouldResemble, some)
	})

	Convey("all", t, func() {
		So(Union(all), ShouldResemble, all)
		So(Union(all, empty), ShouldResemble, all)
		So(Union(all, some1), ShouldResemble, all)
	})
}

func TestExtend(t *testing.T) {
	Convey("Empty", t, func() {
		So(Extend(nil, "user:abc@example.com"), ShouldResemble, &Set{
			IDs: identSet{"user:abc@example.com": struct{}{}},
		})
	})

	Convey("All", t, func() {
		all := &Set{All: true}
		So(Extend(all, "user:abc@example.com"), ShouldResemble, all)
	})

	Convey("Already there", t, func() {
		set, _ := FromStrings([]string{
			"user:abc@example.com",
			"group:abc",
		}, nil)
		So(Extend(set, "user:abc@example.com"), ShouldResemble, set)
	})

	Convey("Extends", t, func() {
		set, _ := FromStrings([]string{
			"user:def@example.com",
			"group:abc",
		}, nil)
		So(Extend(set, "user:abc@example.com"), ShouldResemble, &Set{
			IDs: identSet{
				"user:abc@example.com": struct{}{},
				"user:def@example.com": struct{}{},
			},
			Groups: groupSet{"abc": struct{}{}},
		})
	})
}
