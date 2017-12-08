// Copyright 2017 The LUCI Authors.
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
package acl

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/scheduler/appengine/messages"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
)

func TestAclsValidation(t *testing.T) {
	t.Parallel()

	validGrant := &messages.Acl{Role: messages.Acl_READER, GrantedTo: "group:all"}
	validGrants := []*messages.Acl{validGrant}

	Convey("grant validation", t, func() {
		So(validateGrants(validGrants), ShouldBeNil)

		call := func(acls ...*messages.Acl) error {
			return validateGrants(acls)
		}
		So(call(&messages.Acl{Role: messages.Acl_OWNER, GrantedTo: "e@example.com"}), ShouldBeNil)

		So(call(&messages.Acl{Role: 3}).Error(), ShouldResemble, "invalid role \"3\"")
		So(call(&messages.Acl{Role: messages.Acl_OWNER, GrantedTo: ""}).Error(),
			ShouldResemble, "missing granted_to for role OWNER")
		So(call(&messages.Acl{Role: messages.Acl_OWNER, GrantedTo: "group:"}).Error(),
			ShouldResemble, "invalid granted_to \"group:\" for role OWNER: needs a group name")
		So(call(&messages.Acl{Role: messages.Acl_OWNER, GrantedTo: "not-email"}).Error(),
			ShouldStartWith, "invalid granted_to \"not-email\" for role OWNER: ")
		So(call(&messages.Acl{Role: messages.Acl_OWNER, GrantedTo: "bot:"}).Error(),
			ShouldStartWith, "invalid granted_to \"bot:\" for role OWNER: ")
	})

	validAclSet := &messages.AclSet{Name: "public", Acls: validGrants}

	Convey("Validate AclSets", t, func() {
		as, err := ValidateAclSets([]*messages.AclSet{validAclSet})
		So(err, ShouldBeNil)
		So(len(as), ShouldEqual, 1)
		So(as["public"], ShouldResemble, validGrants)

		shouldError := func(sets ...*messages.AclSet) {
			_, err := ValidateAclSets(sets)
			So(err, ShouldNotBeNil)
		}

		shouldError(&messages.AclSet{Name: "one"})
		shouldError(&messages.AclSet{Name: "?bad i'd", Acls: validGrants})
		shouldError(validAclSet, validAclSet)
	})

	Convey("Task Acls", t, func() {
		Convey("READER and OWNER ACLs are required", func() {
			_, err := ValidateTaskAcls(nil, nil,
				[]*messages.Acl{{Role: messages.Acl_READER, GrantedTo: "group:readers"}})
			So(err.Error(), ShouldResemble, "Job or Trigger must have OWNER acl set")

			_, err = ValidateTaskAcls(nil, nil,
				[]*messages.Acl{{Role: messages.Acl_OWNER, GrantedTo: "group:owners"}})
			So(err.Error(), ShouldResemble, "Job or Trigger must have READER acl set")
		})

		Convey("Without AclSets but with bad ACLs", func() {
			_, err := ValidateTaskAcls(nil, nil, []*messages.Acl{
				{Role: messages.Acl_OWNER, GrantedTo: ""}})
			So(err, ShouldNotBeNil)
		})

		Convey("Many ACLs", func() {
			taskGrants := make([]*messages.Acl, maxGrantsPerJob)
			taskGrants[0] = &messages.Acl{Role: messages.Acl_READER, GrantedTo: "group:readers"}
			for i := 1; i < maxGrantsPerJob; i++ {
				taskGrants[i] = &messages.Acl{Role: messages.Acl_OWNER, GrantedTo: fmt.Sprintf("group:%d", i)}
			}
			So(len(taskGrants), ShouldEqual, maxGrantsPerJob)
			Convey("Hitting max is OK", func() {
				r, err := ValidateTaskAcls(nil, nil, taskGrants)
				So(err, ShouldBeNil)
				So(len(r.Readers), ShouldEqual, 1)
				So(len(r.Owners), ShouldEqual, maxGrantsPerJob-1)
			})
			Convey("1 too many", func() {
				aclSets := map[string][]*messages.Acl{
					"public": {{Role: messages.Acl_READER, GrantedTo: "group:all"}},
				}
				_, err := ValidateTaskAcls(aclSets, []string{"public"}, taskGrants)
				So(err.Error(), ShouldResemble, "Job or Trigger can have at most 32 acls, but 33 given")
			})
		})

		protoAclSets := []*messages.AclSet{
			{Name: "public", Acls: []*messages.Acl{
				{Role: messages.Acl_READER, GrantedTo: "group:all"},
				{Role: messages.Acl_OWNER, GrantedTo: "group:owners"},
			}},
			{Name: "power-owners", Acls: []*messages.Acl{
				{Role: messages.Acl_OWNER, GrantedTo: "group:power"},
			}},
			{Name: "private", Acls: []*messages.Acl{
				{Role: messages.Acl_READER, GrantedTo: "group:internal"},
			}},
		}
		aclSets, err := ValidateAclSets(protoAclSets)
		So(err, ShouldBeNil)
		So(len(aclSets), ShouldEqual, 3)

		Convey("Bad acl_set reference in a task definition", func() {
			_, err := ValidateTaskAcls(aclSets, []string{"typo"}, validGrants)
			So(err.Error(), ShouldResemble, "referencing AclSet 'typo' which doesn't exist")
		})

		Convey("Merging", func() {
			jobAcls, err := ValidateTaskAcls(
				aclSets, []string{"public", "power-owners"},
				[]*messages.Acl{
					{Role: messages.Acl_OWNER, GrantedTo: "me@example.com"},
					{Role: messages.Acl_READER, GrantedTo: "you@example.com"},
				})
			So(err, ShouldBeNil)
			So(jobAcls.Owners, ShouldResemble, []string{"group:owners", "group:power", "me@example.com"})
			So(jobAcls.Readers, ShouldResemble, []string{"group:all", "you@example.com"})

			jobAcls, err = ValidateTaskAcls(
				aclSets, []string{"private"},
				[]*messages.Acl{
					{Role: messages.Acl_OWNER, GrantedTo: "me@example.com"},
				})
			So(err, ShouldBeNil)
			So(jobAcls.Owners, ShouldResemble, []string{"me@example.com"})
			So(jobAcls.Readers, ShouldResemble, []string{"group:internal"})
		})
	})
}

func TestAclsChecks(t *testing.T) {
	t.Parallel()
	ctx := gaetesting.TestingContext()

	basicGroups := GrantsByRole{Owners: []string{"group:owners"}, Readers: []string{"group:readers"}}

	Convey("Admins are owners and readers", t, func() {
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity:       "user:admin@example.com",
			IdentityGroups: []string{"administrators"},
		})
		yup, err := basicGroups.IsOwner(ctx)
		So(err, ShouldBeNil)
		So(yup, ShouldBeTrue)
		yup, err = basicGroups.IsReader(ctx)
		So(err, ShouldBeNil)
		So(yup, ShouldBeTrue)
	})
	Convey("Owners", t, func() {
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity:       "user:owner@example.com",
			IdentityGroups: []string{"owners"},
		})
		yup, err := basicGroups.IsReader(ctx)
		So(err, ShouldBeNil)
		So(yup, ShouldBeTrue)
		yup, err = basicGroups.IsReader(ctx)
		So(err, ShouldBeNil)
		So(yup, ShouldBeTrue)
	})
	Convey("Readers", t, func() {
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity:       "user:reader@example.com",
			IdentityGroups: []string{"readers"},
		})
		nope, err := basicGroups.IsOwner(ctx)
		So(err, ShouldBeNil)
		So(nope, ShouldBeFalse)
		yup, err := basicGroups.IsReader(ctx)
		So(err, ShouldBeNil)
		So(yup, ShouldBeTrue)
	})
	Convey("By email", t, func() {
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity:       "user:reader@example.com",
			IdentityGroups: []string{"all"},
		})
		g := GrantsByRole{
			Owners:  []string{"group:owners"},
			Readers: []string{"group:some", "reader@example.com"},
		}
		nope, err := g.IsOwner(ctx)
		So(err, ShouldBeNil)
		So(nope, ShouldBeFalse)
		yup, err := g.IsReader(ctx)
		So(err, ShouldBeNil)
		So(yup, ShouldBeTrue)
	})
}

func TestAclsEqual(t *testing.T) {
	t.Parallel()
	Convey("GrantsByRole.Equal", t, func() {
		x1 := GrantsByRole{Readers: []string{"a"}, Owners: []string{"b", "c"}}
		x2 := GrantsByRole{Readers: []string{"a"}, Owners: []string{"b", "c"}}
		So(x1.Equal(&x2), ShouldBeTrue)
		y := GrantsByRole{Readers: []string{"e", "g"}, Owners: []string{"b", "d"}}
		z := GrantsByRole{Readers: []string{"e", "g"}, Owners: []string{"b", "c", "d"}}
		So(x1.Equal(&y), ShouldBeFalse)
		So(y.Equal(&z), ShouldBeFalse)
	})
}
