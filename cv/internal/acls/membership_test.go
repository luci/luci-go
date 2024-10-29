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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/caching"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
)

func TestMembership(t *testing.T) {
	t.Parallel()

	ftt.Run("Membership", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)
		const lProject = "test-proj"
		prjcfgtest.Create(ctx, lProject, &cfgpb.Config{
			ConfigGroups: []*cfgpb.ConfigGroup{{
				Name: "main",
			}},
			HonorGerritLinkedAccounts: true,
		})

		makeIdentity := func(email string) identity.Identity {
			id, err := identity.MakeIdentity(fmt.Sprintf("%s:%s", identity.User, email))
			assert.NoErr(t, err)
			return id
		}

		groups := []string{"nooglers", "googlers", "xooglers"}
		ctx = caching.WithEmptyProcessCache(ctx)

		t.Run("no linked accounts", func(t *ftt.Test) {
			const unlinkedEmail = "bar@google.com"
			ct.GFake.AddLinkedAccountMapping([]*gerritpb.EmailInfo{
				{Email: unlinkedEmail},
			})

			t.Run("IsMember returns true when the given identity is authorized", func(t *ftt.Test) {
				ct.AddMember(unlinkedEmail, "googlers")
				ok, err := IsMember(ctx, ct.GFake, "foo", lProject, makeIdentity(unlinkedEmail), groups)
				assert.NoErr(t, err)
				assert.Loosely(t, ok, should.BeTrue)
			})

			t.Run("IsMember returns false when the given identity is unauthorized", func(t *ftt.Test) {
				ok, err := IsMember(ctx, ct.GFake, "foo", lProject, makeIdentity(unlinkedEmail), groups)
				assert.NoErr(t, err)
				assert.Loosely(t, ok, should.BeFalse)
			})
		})

		t.Run("gerrit returns error", func(t *ftt.Test) {
			const email = "foo@google.com"

			t.Run("IsMember returns true when the given identity is authorized", func(t *ftt.Test) {
				ct.AddMember(email, "googlers")
				ok, err := IsMember(ctx, ct.GFake, "foo", lProject, makeIdentity(email), groups)
				assert.NoErr(t, err)
				assert.Loosely(t, ok, should.BeTrue)
			})

			t.Run("IsMember returns false when the given identity is unauthorized", func(t *ftt.Test) {
				ok, err := IsMember(ctx, ct.GFake, "foo", lProject, makeIdentity(email), groups)
				assert.NoErr(t, err)
				assert.Loosely(t, ok, should.BeFalse)
			})
		})

		t.Run("linked accounts", func(t *ftt.Test) {
			const linkedEmail1 = "foo@google.com"
			const linkedEmail2 = "foo@chromium.org"

			ct.GFake.AddLinkedAccountMapping([]*gerritpb.EmailInfo{
				{Email: linkedEmail1},
				{Email: linkedEmail2},
			})

			t.Run("IsMember returns true when the given linked identity is authorized", func(t *ftt.Test) {
				ct.AddMember(linkedEmail1, "googlers")
				ok, err := IsMember(ctx, ct.GFake, "foo", lProject, makeIdentity(linkedEmail1), groups)
				assert.NoErr(t, err)
				assert.Loosely(t, ok, should.BeTrue)
			})

			t.Run("IsMember returns true when the given identity's linked account is authorized", func(t *ftt.Test) {
				ct.AddMember(linkedEmail1, "googlers")
				ok, err := IsMember(ctx, ct.GFake, "foo", lProject, makeIdentity(linkedEmail2), groups)
				assert.NoErr(t, err)
				assert.Loosely(t, ok, should.BeTrue)
			})

			t.Run("IsMember returns false if project doesn't honor linked account even if the given identity's linked account is authorized", func(t *ftt.Test) {
				ct.AddMember(linkedEmail1, "googlers")
				prjcfgtest.Update(ctx, lProject, &cfgpb.Config{
					ConfigGroups: []*cfgpb.ConfigGroup{{
						Name: "main",
					}},
					HonorGerritLinkedAccounts: false,
				})
				ok, err := IsMember(ctx, ct.GFake, "foo", lProject, makeIdentity(linkedEmail2), groups)
				assert.NoErr(t, err)
				assert.Loosely(t, ok, should.BeFalse)
			})

			t.Run("IsMember returns false when all linked accounts are unauthorized", func(t *ftt.Test) {
				ok, err := IsMember(ctx, ct.GFake, "foo", lProject, makeIdentity(linkedEmail2), groups)
				assert.NoErr(t, err)
				assert.Loosely(t, ok, should.BeFalse)
			})

			t.Run("listActiveAccountEmails returns all non-pending linked email addresses", func(t *ftt.Test) {
				emails, err := listActiveAccountEmails(ctx, ct.GFake, "foo", lProject, linkedEmail1)
				assert.NoErr(t, err)
				assert.Loosely(t, emails, should.Resemble([]string{linkedEmail1, linkedEmail2}))
			})

			t.Run("IsMember looks up cache on second hit", func(t *ftt.Test) {
				ct.AddMember(linkedEmail1, "googlers")
				ok, err := IsMember(ctx, ct.GFake, "foo", lProject, makeIdentity(linkedEmail2), groups)
				assert.NoErr(t, err)
				assert.Loosely(t, ok, should.BeTrue)

				ok, err = IsMember(ctx, ct.GFake, "foo", lProject, makeIdentity(linkedEmail2), groups)
				assert.NoErr(t, err)
				assert.Loosely(t, ok, should.BeTrue)
				assert.Loosely(t, ct.GFake.Requests(), should.HaveLength(1))

				t.Run("IsMember calls gerrit after cache expires", func(t *ftt.Test) {
					ct.Clock.Add(cacheTTL + time.Second)
					ok, err = IsMember(ctx, ct.GFake, "foo", lProject, makeIdentity(linkedEmail2), groups)
					assert.NoErr(t, err)
					assert.Loosely(t, ok, should.BeTrue)
					assert.Loosely(t, ct.GFake.Requests(), should.HaveLength(2))
				})
			})
		})

		t.Run("linked accounts pending confirmation", func(t *ftt.Test) {
			const linkedEmail1 = "foo@google.com"
			const linkedEmail2 = "foo@chromium.org"

			ct.GFake.AddLinkedAccountMapping([]*gerritpb.EmailInfo{
				{Email: linkedEmail1},
				{Email: linkedEmail2, PendingConfirmation: true},
			})

			t.Run("IsMember returns true when the given linked identity is authorized", func(t *ftt.Test) {
				ct.AddMember(linkedEmail1, "googlers")
				ok, err := IsMember(ctx, ct.GFake, "foo", lProject, makeIdentity(linkedEmail1), groups)
				assert.NoErr(t, err)
				assert.Loosely(t, ok, should.BeTrue)
			})

			t.Run("IsMember returns false when the linked authorized email is pending_confirmation", func(t *ftt.Test) {
				ct.AddMember(linkedEmail2, "googlers")
				ok, err := IsMember(ctx, ct.GFake, "foo", lProject, makeIdentity(linkedEmail1), groups)
				assert.NoErr(t, err)
				assert.Loosely(t, ok, should.BeFalse)
			})

			t.Run("listActiveAccountEmails skips pending email addresses", func(t *ftt.Test) {
				emails, err := listActiveAccountEmails(ctx, ct.GFake, "foo", lProject, linkedEmail1)
				assert.NoErr(t, err)
				assert.Loosely(t, emails, should.Resemble([]string{linkedEmail1}))
			})
		})
	})
}
