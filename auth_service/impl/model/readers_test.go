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
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/auth_service/api/configspb"
	"go.chromium.org/luci/auth_service/impl/util/gs"
	"go.chromium.org/luci/auth_service/internal/configs/srvcfg/settingscfg"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
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
func getRawReaders(ctx context.Context) []*AuthDBReader {
	q := datastore.NewQuery("AuthDBReader").Ancestor(authDBReadersRootKey(ctx))
	readers := []*AuthDBReader{}
	So(datastore.GetAll(ctx, q, &readers), ShouldBeNil)
	return readers
}

func TestIsAuthorizedReader(t *testing.T) {
	t.Parallel()

	Convey("IsAuthorizedReader works", t, func() {
		ctx := memory.Use(context.Background())

		// Set up an authorized user.
		So(datastore.Put(ctx,
			testReader(ctx, "someone@example.com"),
		), ShouldBeNil)

		Convey("true for authorized user", func() {
			authorized, err := IsAuthorizedReader(ctx, "someone@example.com")
			So(err, ShouldBeNil)
			So(authorized, ShouldBeTrue)
		})

		Convey("false for not authorized user", func() {
			authorized, err := IsAuthorizedReader(ctx, "somebody@example.com")
			So(err, ShouldBeNil)
			So(authorized, ShouldBeFalse)
		})
	})
}

func TestGetAuthorizedEmails(t *testing.T) {
	t.Parallel()

	Convey("GetAuthorizedEmails returns all readers", t, func() {
		ctx := memory.Use(context.Background())

		// No readers.
		readers, err := GetAuthorizedEmails(ctx)
		So(err, ShouldBeNil)
		So(readers, ShouldBeEmpty)

		// A couple of readers.
		So(datastore.Put(ctx,
			testReader(ctx, "adam@example.com"),
			testReader(ctx, "eve@example.com"),
		), ShouldBeNil)
		readers, err = GetAuthorizedEmails(ctx)
		So(err, ShouldBeNil)
		So(readers, ShouldEqual,
			stringset.NewFromSlice("eve@example.com", "adam@example.com"))
	})
}

func TestAuthorizeReader(t *testing.T) {
	t.Parallel()

	Convey("AuthorizeReader works", t, func() {
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
		So(settingscfg.SetConfig(ctx, cfg), ShouldBeNil)

		Convey("disallows long emails", func() {
			testEmail := strings.Repeat("a", MaxReaderEmailLength-12) + "@example.com"
			So(AuthorizeReader(ctx, testEmail), ShouldErrLike,
				"email is too long")
			So(getRawReaders(ctx), ShouldBeEmpty)
		})

		Convey("respects max reader count", func() {
			// Set up lots of existing readers.
			dummyReaders := make([]*AuthDBReader, MaxReaders)
			for i := 0; i < MaxReaders; i++ {
				dummyReaders[i] = testReader(ctx,
					fmt.Sprintf("user-%d@example.com", i))
			}
			So(datastore.Put(ctx, dummyReaders), ShouldBeNil)
			So(getRawReaders(ctx), ShouldHaveLength, MaxReaders)

			// Check authorizing an additional user fails.
			So(AuthorizeReader(ctx, "someone@example.com"), ShouldErrLike,
				"soft limit on GCS ACL entries")
			So(getRawReaders(ctx), ShouldHaveLength, MaxReaders)
		})

		Convey("email is recorded", func() {
			// Define expected client calls.
			gomock.InOrder(
				mockClient.Client.EXPECT().UpdateReadACL(gomock.Any(),
					gomock.Any(), stringset.NewFromSlice("someone@example.com")).Times(2),
				mockClient.Client.EXPECT().Close().Times(1))

			So(AuthorizeReader(ctx, "someone@example.com"), ShouldBeNil)
			So(getRawReaders(ctx), ShouldResembleProto, []*AuthDBReader{
				{
					Kind:         "AuthDBReader",
					Parent:       authDBReadersRootKey(ctx),
					ID:           "someone@example.com",
					AuthorizedTS: testModifiedTS,
				},
			})

			Convey("already authorized user is not duplicated", func() {
				// Define expected client calls.
				gomock.InOrder(
					mockClient.Client.EXPECT().UpdateReadACL(gomock.Any(),
						gomock.Any(), stringset.NewFromSlice("someone@example.com")).Times(2),
					mockClient.Client.EXPECT().Close().Times(1))

				So(AuthorizeReader(ctx, "someone@example.com"), ShouldBeNil)
				So(getRawReaders(ctx), ShouldHaveLength, 1)
			})
		})
	})
}

func TestDeauthorizeReader(t *testing.T) {
	t.Parallel()

	Convey("DeauthorizeReader works", t, func() {
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
		So(settingscfg.SetConfig(ctx, cfg), ShouldBeNil)

		// Add an authorized user.
		So(datastore.Put(ctx, testReader(ctx, "someone@example.com")),
			ShouldBeNil)

		Convey("succeeds for non-authorized user", func() {
			// Define expected client calls.
			gomock.InOrder(
				mockClient.Client.EXPECT().UpdateReadACL(gomock.Any(),
					gomock.Any(), stringset.NewFromSlice("someone@example.com")).Times(2),
				mockClient.Client.EXPECT().Close().Times(1))

			So(DeauthorizeReader(ctx, "unknown@example.com"), ShouldBeNil)
			So(getRawReaders(ctx), ShouldResembleProto, []*AuthDBReader{
				testReader(ctx, "someone@example.com"),
			})
		})

		Convey("removes the user", func() {
			// Define expected client calls.
			gomock.InOrder(
				mockClient.Client.EXPECT().UpdateReadACL(gomock.Any(),
					gomock.Any(), stringset.New(0)).Times(2),
				mockClient.Client.EXPECT().Close().Times(1))

			So(DeauthorizeReader(ctx, "someone@example.com"), ShouldBeNil)
			So(getRawReaders(ctx), ShouldBeEmpty)
		})
	})
}
