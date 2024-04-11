// Copyright 2024 The LUCI Authors.
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

package acls

import (
	"fmt"
	"testing"
	"time"

	"go.chromium.org/luci/auth/identity"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/server/caching"

	"go.chromium.org/luci/cv/internal/cvtesting"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMembership(t *testing.T) {
	// TODO(crbug.com/40287772): remove and uncomment
	linkedAccountAllowedProjects.Add("bar")
	defer linkedAccountAllowedProjects.Del("bar")
	// t.Parallel()

	Convey("Membership", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()

		makeIdentity := func(email string) identity.Identity {
			id, err := identity.MakeIdentity(fmt.Sprintf("%s:%s", identity.User, email))
			So(err, ShouldBeNil)
			return id
		}

		groups := []string{"nooglers", "googlers", "xooglers"}
		ctx = caching.WithEmptyProcessCache(ctx)

		Convey("no linked accounts", func() {
			const unlinkedEmail = "bar@google.com"
			ct.GFake.AddLinkedAccountMapping([]*gerritpb.EmailInfo{
				&gerritpb.EmailInfo{Email: unlinkedEmail},
			})

			Convey("IsMemberLinkedAccounts returns true when the given identity is authorized", func() {
				ct.AddMember(unlinkedEmail, "googlers")
				ok, err := IsMemberLinkedAccounts(ctx, ct.GFake, "foo", "bar", makeIdentity(unlinkedEmail), groups)
				So(err, ShouldBeNil)
				So(ok, ShouldBeTrue)
			})

			Convey("IsMemberLinkedAccounts returns false when the given identity is unauthorized", func() {
				ok, err := IsMemberLinkedAccounts(ctx, ct.GFake, "foo", "bar", makeIdentity(unlinkedEmail), groups)
				So(err, ShouldBeNil)
				So(ok, ShouldBeFalse)
			})
		})

		Convey("linked accounts", func() {
			const linkedEmail1 = "foo@google.com"
			const linkedEmail2 = "foo@chromium.org"

			ct.GFake.AddLinkedAccountMapping([]*gerritpb.EmailInfo{
				&gerritpb.EmailInfo{Email: linkedEmail1},
				&gerritpb.EmailInfo{Email: linkedEmail2},
			})

			Convey("IsMemberLinkedAccounts returns true when the given linked identity is authorized", func() {
				ct.AddMember(linkedEmail1, "googlers")
				ok, err := IsMemberLinkedAccounts(ctx, ct.GFake, "foo", "bar", makeIdentity(linkedEmail1), groups)
				So(err, ShouldBeNil)
				So(ok, ShouldBeTrue)
			})

			Convey("IsMemberLinkedAccounts returns true when the given identity's linked account is authorized", func() {
				ct.AddMember(linkedEmail1, "googlers")
				ok, err := IsMemberLinkedAccounts(ctx, ct.GFake, "foo", "bar", makeIdentity(linkedEmail2), groups)
				So(err, ShouldBeNil)
				So(ok, ShouldBeTrue)
			})

			Convey("IsMemberLinkedAccounts returns false when all linked accounts are unauthorized", func() {
				ok, err := IsMemberLinkedAccounts(ctx, ct.GFake, "foo", "bar", makeIdentity(linkedEmail2), groups)
				So(err, ShouldBeNil)
				So(ok, ShouldBeFalse)
			})

			Convey("listActiveAccountEmails returns all non-pending linked email addresses", func() {
				emails, err := listActiveAccountEmails(ctx, ct.GFake, "foo", "bar", linkedEmail1)
				So(err, ShouldBeNil)
				So(emails, ShouldEqual, []string{linkedEmail1, linkedEmail2})
			})

			Convey("IsMemberLinkedAccounts looks up cache on second hit", func() {
				ct.AddMember(linkedEmail1, "googlers")
				ok, err := IsMemberLinkedAccounts(ctx, ct.GFake, "foo", "bar", makeIdentity(linkedEmail2), groups)
				So(err, ShouldBeNil)
				So(ok, ShouldBeTrue)

				ok, err = IsMemberLinkedAccounts(ctx, ct.GFake, "foo", "bar", makeIdentity(linkedEmail2), groups)
				So(err, ShouldBeNil)
				So(ok, ShouldBeTrue)
				So(ct.GFake.Requests(), ShouldHaveLength, 1)

				Convey("IsMemberLinkedAccounts calls gerrit after cache expires", func() {
					ct.Clock.Add(cacheTTL + time.Second)
					ok, err = IsMemberLinkedAccounts(ctx, ct.GFake, "foo", "bar", makeIdentity(linkedEmail2), groups)
					So(err, ShouldBeNil)
					So(ok, ShouldBeTrue)
					So(ct.GFake.Requests(), ShouldHaveLength, 2)
				})
			})
		})

		Convey("linked accounts pending confirmation", func() {
			const linkedEmail1 = "foo@google.com"
			const linkedEmail2 = "foo@chromium.org"

			ct.GFake.AddLinkedAccountMapping([]*gerritpb.EmailInfo{
				&gerritpb.EmailInfo{Email: linkedEmail1},
				&gerritpb.EmailInfo{Email: linkedEmail2, PendingConfirmation: true},
			})

			Convey("IsMemberLinkedAccounts returns true when the given linked identity is authorized", func() {
				ct.AddMember(linkedEmail1, "googlers")
				ok, err := IsMemberLinkedAccounts(ctx, ct.GFake, "foo", "bar", makeIdentity(linkedEmail1), groups)
				So(err, ShouldBeNil)
				So(ok, ShouldBeTrue)
			})

			Convey("IsMemberLinkedAccounts returns false when the linked authorized email is pending_confirmation", func() {
				ct.AddMember(linkedEmail2, "googlers")
				ok, err := IsMemberLinkedAccounts(ctx, ct.GFake, "foo", "bar", makeIdentity(linkedEmail1), groups)
				So(err, ShouldBeNil)
				So(ok, ShouldBeFalse)
			})

			Convey("listActiveAccountEmails skips pending email addresses", func() {
				emails, err := listActiveAccountEmails(ctx, ct.GFake, "foo", "bar", linkedEmail1)
				So(err, ShouldBeNil)
				So(emails, ShouldEqual, []string{linkedEmail1})
			})
		})
	})
}
