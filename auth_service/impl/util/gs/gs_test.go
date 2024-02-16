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
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/auth/service/protocol"

	"go.chromium.org/luci/auth_service/api/configspb"
	"go.chromium.org/luci/auth_service/internal/configs/srvcfg/settingscfg"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGetPath(t *testing.T) {
	t.Parallel()

	Convey("GetPath works", t, func() {
		ctx := memory.Use(context.Background())

		Convey("error for no settings.cfg", func() {
			_, err := GetPath(ctx)
			So(err, ShouldNotBeNil)
		})

		Convey("empty if not set in settings.cfg", func() {
			// Set up settings config.
			cfg := &configspb.SettingsCfg{}
			So(settingscfg.SetConfig(ctx, cfg), ShouldBeNil)

			gsPath, err := GetPath(ctx)
			So(err, ShouldBeNil)
			So(gsPath, ShouldEqual, "")
		})

		Convey("allows for a single trailing slash", func() {
			// Set up settings config.
			cfg := &configspb.SettingsCfg{
				AuthDbGsPath: "chrome-infra-auth-test.appspot.com/auth-db//",
			}
			So(settingscfg.SetConfig(ctx, cfg), ShouldBeNil)

			gsPath, err := GetPath(ctx)
			So(err, ShouldBeNil)
			So(gsPath, ShouldEqual, "chrome-infra-auth-test.appspot.com/auth-db/")
		})
	})
}

func TestIsValidPath(t *testing.T) {
	t.Parallel()

	Convey("IsValidPath works", t, func() {
		So(IsValidPath(""), ShouldBeFalse)
		So(IsValidPath("path/to/bucket/trailing/slash/"), ShouldBeFalse)
		So(IsValidPath("path/to/bucket"), ShouldBeTrue)
	})
}

func TestGSCalls(t *testing.T) {
	t.Parallel()

	Convey("Google Storage upload and ACL updates work", t, func() {
		ctx := memory.Use(context.Background())

		// Set up mock GS client
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mockClient := NewMockedClient(ctx, ctl)
		ctx = mockClient.Ctx

		// Set up settings config.
		cfg := &configspb.SettingsCfg{
			AuthDbGsPath: "chrome-infra-auth-test.appspot.com/auth-db",
		}
		So(settingscfg.SetConfig(ctx, cfg), ShouldBeNil)

		readers := stringset.NewFromSlice("someone@example.com", "a@b.com")

		Convey("uploading AuthDB works", func() {
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

			// Define expected client calls.
			mockClient.Client.EXPECT().WriteFile(gomock.Any(),
				"chrome-infra-auth-test.appspot.com/auth-db/latest.db",
				"application/protobuf", gomock.Any(), expectedACLs).Times(1)
			mockClient.Client.EXPECT().WriteFile(gomock.Any(),
				"chrome-infra-auth-test.appspot.com/auth-db/latest.json",
				"application/json", gomock.Any(), expectedACLs).Times(1)
			mockClient.Client.EXPECT().Close().Times(1)

			// Upload the AuthDB to GS.
			signedAuthDB := &protocol.SignedAuthDB{}
			rev := &protocol.AuthDBRevision{
				PrimaryId: "chrome-infra-auth-test",
				AuthDbRev: 1234,
			}
			err := UploadAuthDB(ctx, signedAuthDB, rev, readers, false)
			So(err, ShouldBeNil)
		})
	})
}
