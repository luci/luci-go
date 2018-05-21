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
	"testing"

	"github.com/golang/mock/gomock"
	"golang.org/x/net/context"

	"go.chromium.org/gae/impl/memory"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFetchGerritChangeEmail(t *testing.T) {
	t.Parallel()

	Convey("FetchGerritChangeEmail", t, func() {
		c := memory.Use(context.Background())

		ctl := gomock.NewController(t)
		defer ctl.Finish()
		gerritMock := gerritpb.NewMockGerritClient(ctl)
		f := NewMockFactory(ctl)
		f.EXPECT().Gerrit(gomock.Any(), "host", projectUnknownAssumeAllowed).Return(gerritMock, nil)
		c = UseFactory(c, f)

		// Will be called exactly once.
		gerritMock.EXPECT().GetChange(gomock.Any(), gomock.Any()).Return(&gerritpb.ChangeInfo{
			Owner:   &gerritpb.AccountInfo{Email: "user@example.com"},
			Project: "project",
		}, nil)

		f.EXPECT().IsAllowed(gomock.Any(), "host", "project").Return(false, nil)
		_, err := FetchGerritChangeEmail(c, "host", 123)
		Convey("ACLs respected with cold cache", func() {
			So(err.Error(), ShouldContainSubstring, "https://host/123 not found or no access")
		})

		// Now that we have cached change owner, no more GetChange calls should
		// happen, ensured by gerritMock expectation above.

		Convey("ACLs still respected with warm cache", func() {
			f.EXPECT().IsAllowed(gomock.Any(), "host", "project").Return(false, nil)
			_, err = FetchGerritChangeEmail(c, "host", 123)
			So(err.Error(), ShouldContainSubstring, "https://host/123 not found or no access")
		})

		Convey("Happy cached path", func() {
			f.EXPECT().IsAllowed(gomock.Any(), "host", "project").Return(true, nil)
			email, err := FetchGerritChangeEmail(c, "host", 123)
			So(err, ShouldBeNil)
			So(email, ShouldResemble, "user@example.com")
		})
	})
}
