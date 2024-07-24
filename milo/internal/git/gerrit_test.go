// Copyright 2018 The LUCI Authors.
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

package git

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"

	"go.chromium.org/luci/auth/identity"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/milo/internal/git/gitacls"
	configpb "go.chromium.org/luci/milo/proto/config"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/caching"
)

func TestCLEmail(t *testing.T) {
	t.Parallel()

	ftt.Run("CLEmail", t, func(t *ftt.Test) {
		c := caching.WithEmptyProcessCache(context.Background())

		ctl := gomock.NewController(t)
		defer ctl.Finish()
		gerritMock := gerritpb.NewMockGerritClient(ctl)

		host := "limited-review.googlesource.com"
		acls, err := gitacls.FromConfig(c, []*configpb.Settings_SourceAcls{
			{Hosts: []string{"limited.googlesource.com"}, Readers: []string{"allowed@example.com"}},
		})
		assert.Loosely(t, err, should.BeNil)
		impl := implementation{mockGerrit: gerritMock, acls: acls}
		c = Use(c, &impl)
		cAllowed := auth.WithState(c, &authtest.FakeState{Identity: "user:allowed@example.com"})
		cDenied := auth.WithState(c, &authtest.FakeState{Identity: identity.AnonymousIdentity})

		// Will be called exactly once.
		gerritMock.EXPECT().GetChange(gomock.Any(), gomock.Any()).Return(&gerritpb.ChangeInfo{
			Owner:   &gerritpb.AccountInfo{Email: "user@example.com"},
			Project: "project",
		}, nil)

		_, err = impl.CLEmail(cDenied, host, 123)
		t.Run("ACLs respected with cold cache", func(t *ftt.Test) {
			assert.Loosely(t, err.Error(), should.ContainSubstring("not logged in"))
		})

		// Now that we have cached change owner, no more GetChange calls should
		// happen, ensured by gerritMock expectation above.

		t.Run("ACLs still respected with warm cache", func(t *ftt.Test) {
			_, err = impl.CLEmail(cDenied, host, 123)
			assert.Loosely(t, err.Error(), should.ContainSubstring("not logged in"))
		})

		t.Run("Happy cached path", func(t *ftt.Test) {
			email, err := impl.CLEmail(cAllowed, host, 123)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, email, should.Match("user@example.com"))
		})
	})
}
