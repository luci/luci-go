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

package model

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/auth_service/api/configspb"
	"go.chromium.org/luci/auth_service/internal/configs/srvcfg/settingscfg"
	"go.chromium.org/luci/auth_service/internal/gs"
)

func testReader(ctx context.Context, email string) *AuthDBReader {
	return &AuthDBReader{
		Kind:         "AuthDBReader",
		Parent:       authDBReadersRootKey(ctx),
		ID:           email,
		AuthorizedTS: testCreatedTS,
	}
}

// getRawReaders is a helper function to get all AuthDBReaders in
// datastore. MUST be called from within a test case.
func getRawReaders(t testing.TB, ctx context.Context) []*AuthDBReader {
	q := datastore.NewQuery("AuthDBReader").Ancestor(authDBReadersRootKey(ctx))
	readers := []*AuthDBReader{}
	assert.Loosely(t, datastore.GetAll(ctx, q, &readers), should.BeNil)
	return readers
}

func TestIsAuthorizedReader(t *testing.T) {
	t.Parallel()

	ftt.Run("IsAuthorizedReader works", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

		// Set up an authorized user.
		assert.Loosely(t, datastore.Put(ctx,
			testReader(ctx, "someone@example.com"),
		), should.BeNil)

		t.Run("true for authorized user", func(t *ftt.Test) {
			authorized, err := IsAuthorizedReader(ctx, "someone@example.com")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, authorized, should.BeTrue)
		})

		t.Run("false for not authorized user", func(t *ftt.Test) {
			authorized, err := IsAuthorizedReader(ctx, "somebody@example.com")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, authorized, should.BeFalse)
		})
	})
}

func TestGetAuthorizedEmails(t *testing.T) {
	t.Parallel()

	ftt.Run("GetAuthorizedEmails returns all readers", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

		// No readers.
		readers, err := GetAuthorizedEmails(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, readers, should.BeEmpty)

		// A couple of readers.
		assert.Loosely(t, datastore.Put(ctx,
			testReader(ctx, "adam@example.com"),
			testReader(ctx, "eve@example.com"),
		), should.BeNil)
		readers, err = GetAuthorizedEmails(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, readers, should.Match(
			stringset.NewFromSlice("eve@example.com", "adam@example.com")))
	})
}

func TestAuthorizeReader(t *testing.T) {
	t.Parallel()

	ftt.Run("AuthorizeReader works", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx = clock.Set(ctx, testclock.New(testModifiedTS))

		// Set up mock GS client
		ctl := gomock.NewController(t)
		mockClient := gs.NewMockedClient(ctx, ctl)
		ctx = mockClient.Ctx

		// Set up settings config.
		cfg := &configspb.SettingsCfg{
			AuthDbGsPath: "chrome-infra-auth-test.appspot.com/auth-db",
		}
		assert.Loosely(t, settingscfg.SetConfig(ctx, cfg), should.BeNil)

		t.Run("disallows long emails", func(t *ftt.Test) {
			testEmail := strings.Repeat("a", MaxReaderEmailLength-12) + "@example.com"
			assert.Loosely(t, AuthorizeReader(ctx, testEmail), should.ErrLike(
				"email is too long"))
			assert.Loosely(t, getRawReaders(t, ctx), should.BeEmpty)
		})

		t.Run("respects max reader count", func(t *ftt.Test) {
			// Set up lots of existing readers.
			dummyReaders := make([]*AuthDBReader, MaxReaders)
			for i := 0; i < MaxReaders; i++ {
				dummyReaders[i] = testReader(ctx,
					fmt.Sprintf("user-%d@example.com", i))
			}
			assert.Loosely(t, datastore.Put(ctx, dummyReaders), should.BeNil)
			assert.Loosely(t, getRawReaders(t, ctx), should.HaveLength(MaxReaders))

			// Check authorizing an additional user fails.
			assert.Loosely(t, AuthorizeReader(ctx, "someone@example.com"), should.ErrLike(
				"soft limit on GCS ACL entries"))
			assert.Loosely(t, getRawReaders(t, ctx), should.HaveLength(MaxReaders))
		})

		t.Run("email is recorded", func(t *ftt.Test) {
			// Define expected client calls.
			gomock.InOrder(
				mockClient.Client.EXPECT().UpdateReadACL(gomock.Any(),
					gomock.Any(), stringset.NewFromSlice("someone@example.com")).Times(2),
				mockClient.Client.EXPECT().Close().Times(1))

			assert.Loosely(t, AuthorizeReader(ctx, "someone@example.com"), should.BeNil)
			assert.Loosely(t, getRawReaders(t, ctx), should.Match([]*AuthDBReader{
				{
					Kind:         "AuthDBReader",
					Parent:       authDBReadersRootKey(ctx),
					ID:           "someone@example.com",
					AuthorizedTS: testModifiedTS,
				},
			}))

			t.Run("already authorized user is not duplicated", func(t *ftt.Test) {
				// Define expected client calls.
				gomock.InOrder(
					mockClient.Client.EXPECT().UpdateReadACL(gomock.Any(),
						gomock.Any(), stringset.NewFromSlice("someone@example.com")).Times(2),
					mockClient.Client.EXPECT().Close().Times(1))

				assert.Loosely(t, AuthorizeReader(ctx, "someone@example.com"), should.BeNil)
				assert.Loosely(t, getRawReaders(t, ctx), should.HaveLength(1))
			})
		})
	})
}

func TestDeauthorizeReader(t *testing.T) {
	t.Parallel()

	ftt.Run("DeauthorizeReader works", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx = clock.Set(ctx, testclock.New(testModifiedTS))

		// Set up mock GS client
		ctl := gomock.NewController(t)
		mockClient := gs.NewMockedClient(ctx, ctl)
		ctx = mockClient.Ctx

		// Set up settings config.
		cfg := &configspb.SettingsCfg{
			AuthDbGsPath: "chrome-infra-auth-test.appspot.com/auth-db",
		}
		assert.Loosely(t, settingscfg.SetConfig(ctx, cfg), should.BeNil)

		// Add an authorized user.
		assert.Loosely(t, datastore.Put(ctx, testReader(ctx, "someone@example.com")),
			should.BeNil)

		t.Run("succeeds for non-authorized user", func(t *ftt.Test) {
			// Define expected client calls.
			gomock.InOrder(
				mockClient.Client.EXPECT().UpdateReadACL(gomock.Any(),
					gomock.Any(), stringset.NewFromSlice("someone@example.com")).Times(2),
				mockClient.Client.EXPECT().Close().Times(1))

			assert.Loosely(t, DeauthorizeReader(ctx, "unknown@example.com"), should.BeNil)
			assert.Loosely(t, getRawReaders(t, ctx), should.Match([]*AuthDBReader{
				testReader(ctx, "someone@example.com"),
			}))
		})

		t.Run("removes the user", func(t *ftt.Test) {
			// Define expected client calls.
			gomock.InOrder(
				mockClient.Client.EXPECT().UpdateReadACL(gomock.Any(),
					gomock.Any(), stringset.New(0)).Times(2),
				mockClient.Client.EXPECT().Close().Times(1))

			assert.Loosely(t, DeauthorizeReader(ctx, "someone@example.com"), should.BeNil)
			assert.Loosely(t, getRawReaders(t, ctx), should.BeEmpty)
		})
	})
}

func TestRevokeStaleReaderAccess(t *testing.T) {
	t.Parallel()

	ftt.Run("RevokeStaleReaderAccess works", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx = clock.Set(ctx, testclock.New(testModifiedTS))

		// Set up mock GS client
		ctl := gomock.NewController(t)
		mockClient := gs.NewMockedClient(ctx, ctl)
		ctx = mockClient.Ctx

		// Set up settings config.
		cfg := &configspb.SettingsCfg{
			AuthDbGsPath: "chrome-infra-auth-test.appspot.com/auth-db",
		}
		assert.Loosely(t, settingscfg.SetConfig(ctx, cfg), should.BeNil)

		// Add existing readers.
		err := datastore.Put(ctx,
			testReader(ctx, "someone@example.com"),
			testReader(ctx, "somebody@example.com"),
		)
		assert.Loosely(t, err, should.BeNil)

		// The group name for trusted accounts that are eligible to be
		// readers.
		trustedGroup := TrustedServicesGroup

		t.Run("revokes if not trusted", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				FakeDB: authtest.NewFakeDB(
					authtest.MockMembership("user:someone@example.com", trustedGroup),
				),
			})

			// Define expected client calls.
			gomock.InOrder(
				mockClient.Client.EXPECT().UpdateReadACL(gomock.Any(),
					gomock.Any(), stringset.NewFromSlice("someone@example.com")).Times(2),
				mockClient.Client.EXPECT().Close().Times(1))

			assert.Loosely(t, RevokeStaleReaderAccess(ctx, trustedGroup), should.BeNil)
			assert.Loosely(t, getRawReaders(t, ctx), should.Match([]*AuthDBReader{
				testReader(ctx, "someone@example.com"),
			}))
		})

		t.Run("updates ACLs even if there are no deletions", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				FakeDB: authtest.NewFakeDB(
					authtest.MockMembership("user:someone@example.com", trustedGroup),
					authtest.MockMembership("user:somebody@example.com", trustedGroup),
				),
			})

			// Define expected client calls.
			gomock.InOrder(
				mockClient.Client.EXPECT().UpdateReadACL(
					gomock.Any(),
					gomock.Any(),
					stringset.NewFromSlice("someone@example.com", "somebody@example.com"),
				).Times(2),
				mockClient.Client.EXPECT().Close().Times(1))

			assert.Loosely(t, RevokeStaleReaderAccess(ctx, trustedGroup), should.BeNil)
			assert.Loosely(t, getRawReaders(t, ctx), should.Match([]*AuthDBReader{
				testReader(ctx, "somebody@example.com"),
				testReader(ctx, "someone@example.com"),
			}))
		})
	})
}
