// Copyright 2015 The LUCI Authors.
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

package auth

import (
	"testing"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/server/auth/identity"
	. "github.com/smartystreets/goconvey/convey"
)

func TestState(t *testing.T) {
	t.Parallel()

	Convey("Check empty ctx", t, func() {
		ctx := context.Background()
		So(GetState(ctx), ShouldBeNil)
		So(CurrentUser(ctx).Identity, ShouldEqual, identity.AnonymousIdentity)
		So(CurrentIdentity(ctx), ShouldEqual, identity.AnonymousIdentity)

		res, err := IsMember(ctx, "group")
		So(res, ShouldBeFalse)
		So(err, ShouldEqual, ErrNoAuthState)
	})

	Convey("Check non-empty ctx", t, func() {
		s := state{
			db:        &fakeDB{},
			user:      &User{Identity: "user:abc@example.com"},
			peerIdent: "user:abc@example.com",
		}
		ctx := context.WithValue(context.Background(), stateContextKey(0), &s)
		So(GetState(ctx), ShouldNotBeNil)
		So(GetState(ctx).Method(), ShouldBeNil)
		So(GetState(ctx).PeerIdentity(), ShouldEqual, identity.Identity("user:abc@example.com"))
		So(GetState(ctx).PeerIP(), ShouldBeNil)
		So(CurrentUser(ctx).Identity, ShouldEqual, identity.Identity("user:abc@example.com"))
		So(CurrentIdentity(ctx), ShouldEqual, identity.Identity("user:abc@example.com"))

		res, err := IsMember(ctx, "group")
		So(err, ShouldBeNil)
		So(res, ShouldBeTrue) // fakeDB always returns true
	})
}
