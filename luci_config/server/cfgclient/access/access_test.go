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

package access

import (
	"fmt"
	"testing"

	"go.chromium.org/luci/common/auth/identity"
	configPB "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/luci_config/server/cfgclient"
	"go.chromium.org/luci/luci_config/server/cfgclient/backend"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

type testingBackend struct {
	backend.B

	item *backend.Item
}

func (tb *testingBackend) Get(c context.Context, configSet, path string, p backend.Params) (*backend.Item, error) {
	if tb.item == nil {
		return nil, cfgclient.ErrNoConfig
	}
	clone := *tb.item
	return &clone, nil
}

func tpb(msg proto.Message) string { return proto.MarshalTextString(msg) }

func accessCfg(access ...string) string {
	return tpb(&configPB.ProjectCfg{
		Access: access,
	})
}

func TestCheckAccess(t *testing.T) {
	t.Parallel()

	Convey(`A testing environment`, t, func() {
		c := context.Background()

		authState := authtest.FakeState{
			Identity:       identity.AnonymousIdentity,
			IdentityGroups: []string{"all"},
		}
		c = auth.WithState(c, &authState)

		tb := testingBackend{}
		setAccess := func(access ...string) {
			if len(access) == 0 {
				tb.item = nil
				return
			}
			tb.item = &backend.Item{
				Content: tpb(&configPB.ProjectCfg{Access: access}),
			}
		}

		c = backend.WithBackend(c, &tb)

		Convey(`Will grant AsService access to any config`, func() {
			So(Check(c, backend.AsService, "foo/bar"), ShouldBeNil)
			So(Check(c, backend.AsService, "services/foo"), ShouldBeNil)
			So(Check(c, backend.AsService, "projects/nonexistent"), ShouldBeNil)
			So(Check(c, backend.AsService, "projects/public"), ShouldBeNil)
		})

		for _, tc := range []struct {
			A    backend.Authority
			Name string
		}{
			{backend.AsUser, "AsUser"},
			{backend.AsAnonymous, "AsAnonymous"},
		} {
			Convey(fmt.Sprintf(`Will deny %q access to any config`, tc.Name), func() {
				So(Check(c, tc.A, "foo/bar"), ShouldEqual, ErrNoAccess)
				So(Check(c, tc.A, "services/foo"), ShouldEqual, ErrNoAccess)
				So(Check(c, tc.A, "projects/nonexistent"), ShouldErrLike, "failed to load \"project.cfg\"")
			})

			Convey(fmt.Sprintf(`Will grant %q access to an all-inclusive project`, tc.Name), func() {
				setAccess("group:all")
				So(Check(c, tc.A, "projects/public"), ShouldBeNil)
			})
		}

		mustMakeIdentity := func(v string) identity.Identity {
			id, err := identity.MakeIdentity(v)
			if err != nil {
				panic(err)
			}
			return id
		}
		for _, tc := range []struct {
			name     string
			explicit bool
			apply    func()
		}{
			{"a special user", false,
				func() { authState.Identity = mustMakeIdentity("user:cat@example.com") }},
			{"a special user (e-mail)", false,
				func() { authState.Identity = mustMakeIdentity("user:email@example.com") }},
			{"a member of a special group", true,
				func() { authState.IdentityGroups = append(authState.IdentityGroups, "special") }},
		} {
			Convey(fmt.Sprintf(`When user is %s`, tc.name), func() {
				tc.apply()

				setAccess()
				So(Check(c, backend.AsService, "foo/bar"), ShouldBeNil)
				So(Check(c, backend.AsUser, "foo/bar"), ShouldEqual, ErrNoAccess)
				So(Check(c, backend.AsAnonymous, "foo/bar"), ShouldEqual, ErrNoAccess)

				So(Check(c, backend.AsService, "services/foo"), ShouldBeNil)
				So(Check(c, backend.AsUser, "services/foo"), ShouldEqual, ErrNoAccess)
				So(Check(c, backend.AsAnonymous, "services/foo"), ShouldEqual, ErrNoAccess)

				So(Check(c, backend.AsService, "projects/nonexistent"), ShouldBeNil)
				So(Check(c, backend.AsUser, "projects/nonexistent"), ShouldErrLike, "failed to load \"project.cfg\"")
				So(Check(c, backend.AsAnonymous, "projects/nonexistent"), ShouldErrLike, "failed to load \"project.cfg\"")

				setAccess("group:all")
				So(Check(c, backend.AsService, "projects/public"), ShouldBeNil)
				So(Check(c, backend.AsUser, "projects/public"), ShouldBeNil)
				So(Check(c, backend.AsAnonymous, "projects/public"), ShouldBeNil)

				setAccess("group:special", "user:cat@example.com", "email@example.com")
				So(Check(c, backend.AsUser, "projects/exclusive"), ShouldBeNil)
				if tc.explicit {
					So(Check(c, backend.AsAnonymous, "projects/exclusive"), ShouldBeNil)
				} else {
					So(Check(c, backend.AsAnonymous, "projects/exclusive"), ShouldEqual, ErrNoAccess)
				}
			})
		}
	})
}
