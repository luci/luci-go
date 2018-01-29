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

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/config/validation"
	"go.chromium.org/luci/scheduler/appengine/messages"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestACLsValidation(t *testing.T) {
	t.Parallel()

	validGrant := &messages.Acl{Role: messages.Acl_READER, GrantedTo: "group:all"}
	validGrants := []*messages.Acl{validGrant}
	c := gaetesting.TestingContext()

	Convey("grant validation", t, func() {
		call := func(acls ...*messages.Acl) error {
			ctx := &validation.Context{Context: c}
			validateGrants(ctx, acls)
			return ctx.Finalize()
		}
		So(call(validGrant), ShouldBeNil)
		So(call(&messages.Acl{Role: messages.Acl_OWNER, GrantedTo: "e@example.com"}), ShouldBeNil)

		So(call(&messages.Acl{Role: 3}), ShouldErrLike, "invalid role \"3\"")
		So(call(&messages.Acl{Role: messages.Acl_OWNER, GrantedTo: ""}),
			ShouldErrLike, "missing granted_to for role OWNER")
		So(call(&messages.Acl{Role: messages.Acl_OWNER, GrantedTo: "group:"}),
			ShouldErrLike, "invalid granted_to \"group:\" for role OWNER: needs a group name")
		So(call(&messages.Acl{Role: messages.Acl_OWNER, GrantedTo: "not-email"}),
			ShouldErrLike, "invalid granted_to \"not-email\" for role OWNER: ")
		So(call(&messages.Acl{Role: messages.Acl_OWNER, GrantedTo: "bot:"}),
			ShouldErrLike, "invalid granted_to \"bot:\" for role OWNER: ")
	})

	validACLSet := &messages.AclSet{Name: "public", Acls: validGrants}

	Convey("Validate AclSets", t, func() {
		ctx := &validation.Context{Context: c}
		as := ValidateACLSets(ctx, []*messages.AclSet{validACLSet})
		So(ctx.Finalize(), ShouldBeNil)
		So(len(as), ShouldEqual, 1)
		So(as["public"], ShouldResemble, validGrants)

		shouldError := func(sets ...*messages.AclSet) {
			valCtx := &validation.Context{Context: c}
			ValidateACLSets(valCtx, sets)
			So(valCtx.Finalize(), ShouldNotBeNil)
		}

		shouldError(&messages.AclSet{Name: "one"})
		shouldError(&messages.AclSet{Name: "?bad i'd", Acls: validGrants})
		shouldError(validACLSet, validACLSet)
	})

	Convey("Task Acls", t, func() {
		Convey("OWNER ACL is required", func() {
			ctx := &validation.Context{Context: c}
			Convey("No OWNER acl set", func() {
				ValidateTaskACLs(ctx, nil, []string{},
					[]*messages.Acl{{Role: messages.Acl_READER, GrantedTo: "group:readers"}})
				So(ctx.Finalize(), ShouldErrLike,
					"Job or Trigger must have OWNER acl set")
			})
			Convey("No READER is OK", func() {
				ValidateTaskACLs(ctx, nil, []string{},
					[]*messages.Acl{
						{Role: messages.Acl_OWNER, GrantedTo: "group:owners"},
						{Role: messages.Acl_TRIGGERER, GrantedTo: "group:triggerers"},
					})
				So(ctx.Finalize(), ShouldBeNil)
			})
			Convey("No TRIGGERER is OK", func() {
				ValidateTaskACLs(ctx, nil, []string{},
					[]*messages.Acl{
						{Role: messages.Acl_OWNER, GrantedTo: "group:owners"},
						{Role: messages.Acl_READER, GrantedTo: "group:readers"},
					})
				So(ctx.Finalize(), ShouldBeNil)
			})
		})

		Convey("Without AclSets but with bad ACLs", func() {
			ctx := &validation.Context{Context: c}
			ValidateTaskACLs(ctx, nil, []string{}, []*messages.Acl{
				{Role: messages.Acl_OWNER, GrantedTo: ""}})
			So(ctx.Finalize(), ShouldNotBeNil)
		})

		Convey("Many ACLs", func() {
			taskGrants := make([]*messages.Acl, maxGrantsPerJob)
			taskGrants[0] = &messages.Acl{Role: messages.Acl_READER, GrantedTo: "group:readers"}
			for i := 1; i < maxGrantsPerJob; i++ {
				taskGrants[i] = &messages.Acl{Role: messages.Acl_OWNER, GrantedTo: fmt.Sprintf("group:%d", i)}
			}
			So(len(taskGrants), ShouldEqual, maxGrantsPerJob)
			ctx := &validation.Context{Context: c}
			Convey("Hitting max is OK", func() {
				r := ValidateTaskACLs(ctx, nil, []string{}, taskGrants)
				So(ctx.Finalize(), ShouldBeNil)
				So(len(r.Readers), ShouldEqual, 1)
				So(len(r.Owners), ShouldEqual, maxGrantsPerJob-1)
			})
			Convey("1 too many", func() {
				aclSets := map[string][]*messages.Acl{
					"public": {{Role: messages.Acl_TRIGGERER, GrantedTo: "group:triggerers"}},
				}
				ValidateTaskACLs(ctx, aclSets, []string{"public"}, taskGrants)
				So(ctx.Finalize(), ShouldErrLike, "Job or Trigger can have at most 32 acls, but 33 given")
			})
		})

		ctx := &validation.Context{Context: c}
		protoACLSets := []*messages.AclSet{
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
			{Name: "triggeres", Acls: []*messages.Acl{
				{Role: messages.Acl_TRIGGERER, GrantedTo: "group:triggerers"},
			}},
		}
		aclSets := ValidateACLSets(ctx, protoACLSets)
		So(ctx.Finalize(), ShouldBeNil)
		So(len(aclSets), ShouldEqual, 4)

		Convey("Bad acl_set reference in a task definition", func() {
			valCtx := &validation.Context{Context: c}
			ValidateTaskACLs(valCtx, aclSets, []string{"typo"}, validGrants)
			So(valCtx.Finalize(), ShouldErrLike, "(acl_sets): referencing AclSet \"typo\" which doesn't exist")
		})

		Convey("Merging", func() {
			valCtx := &validation.Context{Context: c}
			Convey("ok1", func() {
				jobAcls := ValidateTaskACLs(valCtx,
					aclSets, []string{"public", "power-owners"},
					[]*messages.Acl{
						{Role: messages.Acl_OWNER, GrantedTo: "me@example.com"},
						{Role: messages.Acl_READER, GrantedTo: "you@example.com"},
					})
				So(valCtx.Finalize(), ShouldBeNil)
				So(jobAcls.Owners, ShouldResemble, []string{"group:owners", "group:power", "me@example.com"})
				So(jobAcls.Readers, ShouldResemble, []string{"group:all", "you@example.com"})
			})
			Convey("ok2", func() {
				jobAcls := ValidateTaskACLs(valCtx,
					aclSets, []string{"private"},
					[]*messages.Acl{
						{Role: messages.Acl_OWNER, GrantedTo: "me@example.com"},
					})
				So(valCtx.Finalize(), ShouldBeNil)
				So(jobAcls.Owners, ShouldResemble, []string{"me@example.com"})
				So(jobAcls.Readers, ShouldResemble, []string{"group:internal"})
			})
			Convey("ok3", func() {
				jobAcls := ValidateTaskACLs(valCtx,
					aclSets, []string{"public", "triggeres"},
					[]*messages.Acl{
						{Role: messages.Acl_TRIGGERER, GrantedTo: "triggerer@example.com"},
					})
				So(valCtx.Finalize(), ShouldBeNil)
				So(jobAcls.Owners, ShouldResemble, []string{"group:owners"})
				So(jobAcls.Triggerers, ShouldResemble, []string{"group:triggerers", "triggerer@example.com"})
				So(jobAcls.Readers, ShouldResemble, []string{"group:all"})
			})
		})
	})
}

func TestAclsChecks(t *testing.T) {
	t.Parallel()
	ctx := gaetesting.TestingContext()

	computeRoles := func(ctx context.Context, g GrantsByRole) map[Role]bool {
		r := map[Role]bool{}
		for _, role := range []Role{Reader, Triggerer, Owner} {
			switch granted, err := g.CallerHasRole(ctx, role); {
			case err != nil:
				panic(err)
			default:
				r[role] = granted
			}
		}
		return r
	}

	basicGroups := GrantsByRole{
		Owners:     []string{"group:owners"},
		Triggerers: []string{"group:triggerers"},
		Readers:    []string{"group:readers"},
	}

	Convey("Owners", t, func() {
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity:       "user:owner@example.com",
			IdentityGroups: []string{"owners"},
		})
		So(computeRoles(ctx, basicGroups), ShouldResemble, map[Role]bool{
			Reader:    true,
			Triggerer: true,
			Owner:     true,
		})
	})

	Convey("Readers", t, func() {
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity:       "user:reader@example.com",
			IdentityGroups: []string{"readers"},
		})
		So(computeRoles(ctx, basicGroups), ShouldResemble, map[Role]bool{
			Reader:    true,
			Triggerer: false,
			Owner:     false,
		})
	})

	Convey("Triggerers", t, func() {
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity:       "user:triggerer@example.com",
			IdentityGroups: []string{"triggerers"},
		})
		So(computeRoles(ctx, basicGroups), ShouldResemble, map[Role]bool{
			Reader:    true,
			Triggerer: true,
			Owner:     false,
		})
	})

	Convey("By email", t, func() {
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity:       "user:readers@example.com",
			IdentityGroups: []string{"all"},
		})
		g := GrantsByRole{
			Owners:  []string{"group:owners"},
			Readers: []string{"group:all", "reader@example.com"},
		}
		So(computeRoles(ctx, g), ShouldResemble, map[Role]bool{
			Reader:    true,
			Triggerer: false,
			Owner:     false,
		})
	})
}

func TestAclsEqual(t *testing.T) {
	t.Parallel()
	Convey("GrantsByRole.Equal", t, func() {
		x1 := GrantsByRole{Readers: []string{"a"}, Owners: []string{"b", "c"}, Triggerers: []string{"t"}}
		x2 := GrantsByRole{Readers: []string{"a"}, Owners: []string{"b", "c"}, Triggerers: []string{"t"}}
		So(x1.Equal(&x2), ShouldBeTrue)
		y := GrantsByRole{Readers: []string{"e", "g"}, Owners: []string{"b", "d"}}
		yt := GrantsByRole{Readers: []string{"e", "g"}, Owners: []string{"b", "d"}, Triggerers: []string{"t"}}
		z := GrantsByRole{Readers: []string{"e", "g"}, Owners: []string{"b", "c", "d"}}
		So(x1.Equal(&y), ShouldBeFalse)
		So(y.Equal(&yt), ShouldBeFalse)
		So(y.Equal(&z), ShouldBeFalse)
	})
}
