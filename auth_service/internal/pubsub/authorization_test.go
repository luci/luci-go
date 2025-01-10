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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
)

func TestIsAuthorized(t *testing.T) {
	t.Parallel()

	ftt.Run("IsAuthorizedSubscriber works", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

		// Set up mock Pubsub client
		ctl := gomock.NewController(t)
		mockClient := NewMockedClient(ctx, ctl)
		ctx = mockClient.Ctx

		policy := StubPolicy("user:someone@example.com")

		t.Run("true for authorized account", func(t *ftt.Test) {
			// Define expected client calls.
			gomock.InOrder(
				mockClient.Client.EXPECT().GetIAMPolicy(gomock.Any()).Return(policy, nil).Times(1),
				mockClient.Client.EXPECT().Close().Times(1),
			)

			authorized, err := IsAuthorizedSubscriber(ctx, "someone@example.com")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, authorized, should.BeTrue)
		})

		t.Run("false for unauthorized account", func(t *ftt.Test) {
			// Define expected client calls.
			gomock.InOrder(
				mockClient.Client.EXPECT().GetIAMPolicy(gomock.Any()).Return(policy, nil).Times(1),
				mockClient.Client.EXPECT().Close().Times(1),
			)

			authorized, err := IsAuthorizedSubscriber(ctx, "somebody@example.com")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, authorized, should.BeFalse)
		})
	})
}

func TestAuthorizeSubscriber(t *testing.T) {
	t.Parallel()

	ftt.Run("AuthorizeSubscriber works", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

		// Set up mock Pubsub client
		ctl := gomock.NewController(t)
		mockClient := NewMockedClient(ctx, ctl)
		ctx = mockClient.Ctx

		policy := StubPolicy("user:someone@example.com")

		t.Run("exits early for authorized account", func(t *ftt.Test) {
			// Define expected client calls.
			gomock.InOrder(
				mockClient.Client.EXPECT().GetIAMPolicy(gomock.Any()).Return(policy, nil).Times(1),
				mockClient.Client.EXPECT().Close().Times(1),
			)

			assert.Loosely(t, AuthorizeSubscriber(ctx, "someone@example.com"), should.BeNil)
		})

		t.Run("authorizes account", func(t *ftt.Test) {
			// Define expected client calls.
			gomock.InOrder(
				mockClient.Client.EXPECT().GetIAMPolicy(gomock.Any()).Return(policy, nil).Times(1),
				mockClient.Client.EXPECT().SetIAMPolicy(gomock.Any(), gomock.Any()).Times(1),
				mockClient.Client.EXPECT().Close().Times(1),
			)

			assert.Loosely(t, AuthorizeSubscriber(ctx, "somebody@example.com"), should.BeNil)
		})
	})
}

func TestDeuthorizeSubscriber(t *testing.T) {
	t.Parallel()

	ftt.Run("DeauthorizeSubscriber works", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

		// Set up mock Pubsub client
		ctl := gomock.NewController(t)
		mockClient := NewMockedClient(ctx, ctl)
		ctx = mockClient.Ctx

		policy := StubPolicy("user:someone@example.com")

		t.Run("deauthorizes account", func(t *ftt.Test) {
			// Define expected client calls.
			gomock.InOrder(
				mockClient.Client.EXPECT().GetIAMPolicy(gomock.Any()).Return(policy, nil).Times(1),
				mockClient.Client.EXPECT().SetIAMPolicy(gomock.Any(), gomock.Any()).Times(1),
				mockClient.Client.EXPECT().Close().Times(1),
			)

			assert.Loosely(t, DeauthorizeSubscriber(ctx, "someone@example.com"), should.BeNil)
		})

		t.Run("exits early for unauthorized account", func(t *ftt.Test) {
			// Define expected client calls.
			gomock.InOrder(
				mockClient.Client.EXPECT().GetIAMPolicy(gomock.Any()).Return(policy, nil).Times(1),
				mockClient.Client.EXPECT().Close().Times(1),
			)

			assert.Loosely(t, DeauthorizeSubscriber(ctx, "somebody@example.com"), should.BeNil)
		})
	})
}

func TestRevokeStaleAuthorization(t *testing.T) {
	t.Parallel()

	ftt.Run("RevokeStaleAuthorization works", t, func(t *ftt.Test) {
		// Set up mock Pubsub client
		ctl := gomock.NewController(t)
		mockClient := NewMockedClient(context.Background(), ctl)
		ctx := mockClient.Ctx

		trustedGroup := "trusted-group-name"

		t.Run("revokes if not trusted", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				FakeDB: authtest.NewFakeDB(
					authtest.MockMembership("user:someone@example.com", trustedGroup),
				),
			})

			// Define expected client calls.
			policy := StubPolicy("user:old.trusted@example.com")
			gomock.InOrder(
				mockClient.Client.EXPECT().GetIAMPolicy(gomock.Any()).Return(policy, nil).Times(1),
				mockClient.Client.EXPECT().SetIAMPolicy(gomock.Any(), gomock.Any()).Times(1),
				mockClient.Client.EXPECT().Close().Times(1),
			)

			assert.Loosely(t, RevokeStaleAuthorization(ctx, trustedGroup), should.BeNil)
		})

		t.Run("revokes if deleted, even if trusted", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				FakeDB: authtest.NewFakeDB(
					authtest.MockMembership("user:project-name@example.com", trustedGroup),
				),
			})

			// Define expected client calls.
			policy := StubPolicy("deleted:serviceAccount:project-name@example.com")
			gomock.InOrder(
				mockClient.Client.EXPECT().GetIAMPolicy(gomock.Any()).Return(policy, nil).Times(1),
				mockClient.Client.EXPECT().SetIAMPolicy(gomock.Any(), gomock.Any()).Times(1),
				mockClient.Client.EXPECT().Close().Times(1),
			)

			assert.Loosely(t, RevokeStaleAuthorization(ctx, trustedGroup), should.BeNil)
		})

		t.Run("skips policy update if no changes required", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				FakeDB: authtest.NewFakeDB(
					authtest.MockMembership("user:old.trusted@example.com", trustedGroup),
				),
			})

			// Define expected client calls.
			policy := StubPolicy("user:old.trusted@example.com")
			gomock.InOrder(
				mockClient.Client.EXPECT().GetIAMPolicy(gomock.Any()).Return(policy, nil).Times(1),
				mockClient.Client.EXPECT().Close().Times(1),
			)

			assert.Loosely(t, RevokeStaleAuthorization(ctx, trustedGroup), should.BeNil)
		})
	})
}
