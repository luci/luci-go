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

package pubsub

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	. "github.com/smartystreets/goconvey/convey"
)

func TestIsAuthorized(t *testing.T) {
	t.Parallel()

	Convey("IsAuthorizedSubscriber works", t, func() {
		ctx := memory.Use(context.Background())

		// Set up mock Pubsub client
		ctl := gomock.NewController(t)
		mockClient := NewMockedClient(ctx, ctl)
		ctx = mockClient.Ctx

		policy := StubPolicy("someone@example.com")

		Convey("true for authorized account", func() {
			// Define expected client calls.
			gomock.InOrder(
				mockClient.Client.EXPECT().GetIAMPolicy(gomock.Any()).Return(policy, nil).Times(1),
				mockClient.Client.EXPECT().Close().Times(1),
			)

			authorized, err := IsAuthorizedSubscriber(ctx, "someone@example.com")
			So(err, ShouldBeNil)
			So(authorized, ShouldBeTrue)
		})

		Convey("false for unauthorized account", func() {
			// Define expected client calls.
			gomock.InOrder(
				mockClient.Client.EXPECT().GetIAMPolicy(gomock.Any()).Return(policy, nil).Times(1),
				mockClient.Client.EXPECT().Close().Times(1),
			)

			authorized, err := IsAuthorizedSubscriber(ctx, "somebody@example.com")
			So(err, ShouldBeNil)
			So(authorized, ShouldBeFalse)
		})
	})
}

func TestAuthorizeSubscriber(t *testing.T) {
	t.Parallel()

	Convey("AuthorizeSubscriber works", t, func() {
		ctx := memory.Use(context.Background())

		// Set up mock Pubsub client
		ctl := gomock.NewController(t)
		mockClient := NewMockedClient(ctx, ctl)
		ctx = mockClient.Ctx

		policy := StubPolicy("someone@example.com")

		Convey("exits early for authorized account", func() {
			// Define expected client calls.
			gomock.InOrder(
				mockClient.Client.EXPECT().GetIAMPolicy(gomock.Any()).Return(policy, nil).Times(1),
				mockClient.Client.EXPECT().Close().Times(1),
			)

			So(AuthorizeSubscriber(ctx, "someone@example.com"), ShouldBeNil)
		})

		Convey("authorizes account", func() {
			// Define expected client calls.
			gomock.InOrder(
				mockClient.Client.EXPECT().GetIAMPolicy(gomock.Any()).Return(policy, nil).Times(1),
				mockClient.Client.EXPECT().SetIAMPolicy(gomock.Any(), gomock.Any()).Times(1),
				mockClient.Client.EXPECT().Close().Times(1),
			)

			So(AuthorizeSubscriber(ctx, "somebody@example.com"), ShouldBeNil)
		})
	})
}

func TestDeuthorizeSubscriber(t *testing.T) {
	t.Parallel()

	Convey("DeauthorizeSubscriber works", t, func() {
		ctx := memory.Use(context.Background())

		// Set up mock Pubsub client
		ctl := gomock.NewController(t)
		mockClient := NewMockedClient(ctx, ctl)
		ctx = mockClient.Ctx

		policy := StubPolicy("someone@example.com")

		Convey("deauthorizes account", func() {
			// Define expected client calls.
			gomock.InOrder(
				mockClient.Client.EXPECT().GetIAMPolicy(gomock.Any()).Return(policy, nil).Times(1),
				mockClient.Client.EXPECT().SetIAMPolicy(gomock.Any(), gomock.Any()).Times(1),
				mockClient.Client.EXPECT().Close().Times(1),
			)

			So(DeauthorizeSubscriber(ctx, "someone@example.com"), ShouldBeNil)
		})

		Convey("exits early for unauthorized account", func() {
			// Define expected client calls.
			gomock.InOrder(
				mockClient.Client.EXPECT().GetIAMPolicy(gomock.Any()).Return(policy, nil).Times(1),
				mockClient.Client.EXPECT().Close().Times(1),
			)

			So(DeauthorizeSubscriber(ctx, "somebody@example.com"), ShouldBeNil)
		})
	})
}

func TestRevokeStaleAuthorization(t *testing.T) {
	t.Parallel()

	Convey("RevokeStaleAuthorization works", t, func() {
		// Set up mock Pubsub client
		ctl := gomock.NewController(t)
		mockClient := NewMockedClient(context.Background(), ctl)
		ctx := mockClient.Ctx

		policy := StubPolicy("old.trusted@example.com")
		trustedGroup := "trusted-group-name"

		Convey("revokes if not trusted", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				FakeDB: authtest.NewFakeDB(
					authtest.MockMembership("user:someone@example.com", trustedGroup),
				),
			})

			// Define expected client calls.
			gomock.InOrder(
				mockClient.Client.EXPECT().GetIAMPolicy(gomock.Any()).Return(policy, nil).Times(1),
				mockClient.Client.EXPECT().SetIAMPolicy(gomock.Any(), gomock.Any()).Times(1),
				mockClient.Client.EXPECT().Close().Times(1),
			)

			So(RevokeStaleAuthorization(ctx, trustedGroup, false), ShouldBeNil)
		})

		Convey("skips policy update if no changes required", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				FakeDB: authtest.NewFakeDB(
					authtest.MockMembership("user:old.trusted@example.com", trustedGroup),
				),
			})

			// Define expected client calls.
			gomock.InOrder(
				mockClient.Client.EXPECT().GetIAMPolicy(gomock.Any()).Return(policy, nil).Times(1),
				mockClient.Client.EXPECT().Close().Times(1),
			)

			So(RevokeStaleAuthorization(ctx, trustedGroup, false), ShouldBeNil)
		})
	})
}
