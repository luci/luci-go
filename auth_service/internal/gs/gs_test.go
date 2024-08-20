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

package gs

import (
	"context"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/golang/mock/gomock"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/auth/service/protocol"

	"go.chromium.org/luci/auth_service/api/configspb"
	"go.chromium.org/luci/auth_service/internal/configs/srvcfg/settingscfg"
)

func TestGetPath(t *testing.T) {
	t.Parallel()

	ftt.Run("GetPath works", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

		t.Run("error for no settings.cfg", func(t *ftt.Test) {
			_, err := GetPath(ctx)
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("empty if not set in settings.cfg", func(t *ftt.Test) {
			// Set up settings config.
			cfg := &configspb.SettingsCfg{}
			assert.Loosely(t, settingscfg.SetConfig(ctx, cfg), should.BeNil)

			gsPath, err := GetPath(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, gsPath, should.BeEmpty)
		})

		t.Run("allows for a single trailing slash", func(t *ftt.Test) {
			// Set up settings config.
			cfg := &configspb.SettingsCfg{
				AuthDbGsPath: "chrome-infra-auth-test.appspot.com/auth-db//",
			}
			assert.Loosely(t, settingscfg.SetConfig(ctx, cfg), should.BeNil)

			gsPath, err := GetPath(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, gsPath, should.Equal("chrome-infra-auth-test.appspot.com/auth-db/"))
		})
	})
}

func TestIsValidPath(t *testing.T) {
	t.Parallel()

	ftt.Run("IsValidPath works", t, func(t *ftt.Test) {
		assert.Loosely(t, IsValidPath(""), should.BeFalse)
		assert.Loosely(t, IsValidPath("path/to/bucket/trailing/slash/"), should.BeFalse)
		assert.Loosely(t, IsValidPath("path/to/bucket"), should.BeTrue)
	})
}

func TestUploadAuthDB(t *testing.T) {
	t.Parallel()

	ftt.Run("Uploading to Google Storage works", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		// Set up mock GS client
		ctl := gomock.NewController(t)
		mockClient := NewMockedClient(ctx, ctl)
		ctx = mockClient.Ctx

		signedAuthDB := &protocol.SignedAuthDB{}
		rev := &protocol.AuthDBRevision{
			PrimaryId: "chrome-infra-auth-test",
			AuthDbRev: 1234,
		}
		readers := stringset.NewFromSlice("someone@example.com", "a@b.com")

		t.Run("exits early if not configured", func(t *ftt.Test) {
			// Set up settings config with no GS path.
			assert.Loosely(t, settingscfg.SetConfig(ctx, &configspb.SettingsCfg{}), should.BeNil)
			// There should be no client calls.
			assert.Loosely(t, UploadAuthDB(ctx, signedAuthDB, rev, readers, false), should.BeNil)
		})

		t.Run("uploads AuthDB and revision", func(t *ftt.Test) {
			// Set up settings config.
			cfg := &configspb.SettingsCfg{
				AuthDbGsPath: "chrome-infra-auth-test.appspot.com/auth-db",
			}
			assert.Loosely(t, settingscfg.SetConfig(ctx, cfg), should.BeNil)

			// Define expected client calls.
			expectedACLs := []storage.ACLRule{
				{
					Entity: storage.ACLEntity("user-a@b.com"),
					Role:   storage.RoleReader,
				},
				{
					Entity: storage.ACLEntity("user-someone@example.com"),
					Role:   storage.RoleReader,
				},
			}
			dbWrite := mockClient.Client.EXPECT().WriteFile(gomock.Any(),
				"chrome-infra-auth-test.appspot.com/auth-db/latest.db",
				"application/protobuf", gomock.Any(), expectedACLs).Times(1)
			revWrite := mockClient.Client.EXPECT().WriteFile(gomock.Any(),
				"chrome-infra-auth-test.appspot.com/auth-db/latest.json",
				"application/json", gomock.Any(), expectedACLs).Times(1)
			mockClient.Client.EXPECT().Close().Times(1).After(dbWrite).After(revWrite)

			// Upload the AuthDB to GS.
			assert.Loosely(t, UploadAuthDB(ctx, signedAuthDB, rev, readers, false), should.BeNil)
		})
	})
}

func TestUpdateReaders(t *testing.T) {
	t.Parallel()

	ftt.Run("Google Storage ACL updates work", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		// Set up mock GS client
		ctl := gomock.NewController(t)
		mockClient := NewMockedClient(ctx, ctl)
		ctx = mockClient.Ctx

		readers := stringset.NewFromSlice("someone@example.com", "a@b.com")
		t.Run("exits early if not configured", func(t *ftt.Test) {
			// Set up settings config with no GS path.
			assert.Loosely(t, settingscfg.SetConfig(ctx, &configspb.SettingsCfg{}), should.BeNil)
			// There should be no client calls.
			assert.Loosely(t, UpdateReaders(ctx, readers), should.BeNil)
		})

		t.Run("updates both AuthDB and rev ACLs", func(t *ftt.Test) {
			// Set up settings config.
			cfg := &configspb.SettingsCfg{
				AuthDbGsPath: "chrome-infra-auth-test.appspot.com/auth-db",
			}
			assert.Loosely(t, settingscfg.SetConfig(ctx, cfg), should.BeNil)

			// Define expected client calls.
			dbUpdate := mockClient.Client.EXPECT().UpdateReadACL(gomock.Any(),
				"chrome-infra-auth-test.appspot.com/auth-db/latest.db",
				readers).Times(1)
			revUpdate := mockClient.Client.EXPECT().UpdateReadACL(gomock.Any(),
				"chrome-infra-auth-test.appspot.com/auth-db/latest.json",
				readers).Times(1)
			mockClient.Client.EXPECT().Close().Times(1).After(dbUpdate).After(revUpdate)

			assert.Loosely(t, UpdateReaders(ctx, readers), should.BeNil)
		})
	})
}
