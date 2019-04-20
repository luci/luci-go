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

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/luci/auth/identity"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/milo/api/config"
	"go.chromium.org/luci/milo/git/gitacls"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCLEmail(t *testing.T) {
	t.Parallel()

	Convey("CLEmail", t, func() {
		c := memory.Use(context.Background())

		ctl := gomock.NewController(t)
		defer ctl.Finish()
		gerritMock := gerritpb.NewMockGerritClient(ctl)

		host := "limited-review.googlesource.com"
		acls, err := gitacls.FromConfig(c, []*config.Settings_SourceAcls{
			{Hosts: []string{"limited.googlesource.com"}, Readers: []string{"allowed@example.com"}},
		})
		So(err, ShouldBeNil)
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
		Convey("ACLs respected with cold cache", func() {
			So(err.Error(), ShouldContainSubstring, "not logged in")
		})

		// Now that we have cached change owner, no more GetChange calls should
		// happen, ensured by gerritMock expectation above.

		Convey("ACLs still respected with warm cache", func() {
			_, err = impl.CLEmail(cDenied, host, 123)
			So(err.Error(), ShouldContainSubstring, "not logged in")
		})

		Convey("Happy cached path", func() {
			email, err := impl.CLEmail(cAllowed, host, 123)
			So(err, ShouldBeNil)
			So(email, ShouldResemble, "user@example.com")
		})
	})
}
