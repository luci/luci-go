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

package subscription

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/auth_service/api/configspb"
	"go.chromium.org/luci/auth_service/impl/model"
	"go.chromium.org/luci/auth_service/internal/configs/srvcfg/settingscfg"
	"go.chromium.org/luci/auth_service/internal/gs"
	"go.chromium.org/luci/auth_service/internal/pubsub"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

var (
	testModifiedTS = time.Date(2021, time.August, 16, 12, 20, 0, 0, time.UTC)
)

func TestCheckAccess(t *testing.T) {
	t.Parallel()

	Convey("CheckAccess works", t, func() {
		ctx := memory.Use(context.Background())
		ctx = clock.Set(ctx, testclock.New(testModifiedTS))

		// Set up mock Pubsub client
		ctl := gomock.NewController(t)
		mockPubsubClient := pubsub.NewMockedClient(ctx, ctl)
		ctx = mockPubsubClient.Ctx
		policy := pubsub.StubPolicy("someone@example.com")

		// Set up settings config.
		cfg := &configspb.SettingsCfg{}
		So(settingscfg.SetConfig(ctx, cfg), ShouldBeNil)

		// Set up an authorized user.
		So(model.AuthorizeReader(ctx, "someone@example.com"), ShouldBeNil)

		Convey("user must use email-based auth", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			rw := httptest.NewRecorder()
			rctx := &router.Context{
				Request: (&http.Request{}).WithContext(ctx),
				Writer:  rw,
			}
			err := CheckAccess(rctx)
			So(err, ShouldErrLike, "error getting caller email")
			So(status.Code(err), ShouldEqual, codes.InvalidArgument)
			So(rw.Body.Bytes(), ShouldBeEmpty)
		})

		Convey("false for unauthorized", func() {
			// Set expected Pubsub client calls.
			gomock.InOrder(
				mockPubsubClient.Client.EXPECT().GetIAMPolicy(gomock.Any()).Return(policy, nil).Times(1),
				mockPubsubClient.Client.EXPECT().Close().Times(1))

			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:somebody@example.com",
			})
			rw := httptest.NewRecorder()
			rctx := &router.Context{
				Request: (&http.Request{}).WithContext(ctx),
				Writer:  rw,
			}
			err := CheckAccess(rctx)
			So(err, ShouldBeNil)
			expectedBlob := []byte(`{"topic":"projects/app/topics/auth-db-changed","authorized":false,"gs":{"auth_db_gs_path":"","authorized":false}}`)
			So(rw.Body.Bytes(), ShouldEqual, expectedBlob)
		})

		Convey("true for authorized", func() {
			// Set expected Pubsub client calls.
			gomock.InOrder(
				mockPubsubClient.Client.EXPECT().GetIAMPolicy(gomock.Any()).Return(policy, nil).Times(1),
				mockPubsubClient.Client.EXPECT().Close().Times(1))

			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
			})
			rw := httptest.NewRecorder()
			rctx := &router.Context{
				Request: (&http.Request{}).WithContext(ctx),
				Writer:  rw,
			}
			err := CheckAccess(rctx)
			So(err, ShouldBeNil)
			expectedBlob := []byte(`{"topic":"projects/app/topics/auth-db-changed","authorized":true,"gs":{"auth_db_gs_path":"","authorized":true}}`)
			So(rw.Body.Bytes(), ShouldEqual, expectedBlob)
		})
	})
}

func TestAuthorize(t *testing.T) {
	t.Parallel()

	Convey("Authorize works", t, func() {
		ctx := memory.Use(context.Background())
		ctx = clock.Set(ctx, testclock.New(testModifiedTS))

		// Set up mock Pubsub and GS client
		ctl := gomock.NewController(t)
		mockGSClient := gs.NewMockedClient(ctx, ctl)
		mockPubsubClient := pubsub.NewMockedClient(mockGSClient.Ctx, ctl)
		ctx = mockPubsubClient.Ctx

		// Set up settings config.
		cfg := &configspb.SettingsCfg{
			AuthDbGsPath: "chrome-infra-auth-test.appspot.com/auth-db",
		}
		So(settingscfg.SetConfig(ctx, cfg), ShouldBeNil)

		Convey("user must use email-based auth", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			rw := httptest.NewRecorder()
			rctx := &router.Context{
				Request: (&http.Request{}).WithContext(ctx),
				Writer:  rw,
			}
			err := Authorize(rctx)
			So(err, ShouldErrLike, "error getting caller email")
			So(status.Code(err), ShouldEqual, codes.InvalidArgument)
			So(rw.Body.Bytes(), ShouldBeEmpty)
		})

		Convey("authorizes a new user", func() {
			// Set expected GS client calls from updating ACLs.
			gomock.InOrder(
				mockGSClient.Client.EXPECT().UpdateReadACL(
					gomock.Any(), gomock.Any(), stringset.NewFromSlice("someone@example.com")).Times(2),
				mockGSClient.Client.EXPECT().Close().Times(1))

			// Set expected Pubsub client calls.
			gomock.InOrder(
				mockPubsubClient.Client.EXPECT().GetIAMPolicy(gomock.Any()).Return(pubsub.StubPolicy(), nil).Times(1),
				mockPubsubClient.Client.EXPECT().SetIAMPolicy(gomock.Any(), gomock.Any()).Times(1),
				mockPubsubClient.Client.EXPECT().Close().Times(1))

			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
			})
			rw := httptest.NewRecorder()
			rctx := &router.Context{
				Request: (&http.Request{}).WithContext(ctx),
				Writer:  rw,
			}
			err := Authorize(rctx)
			So(err, ShouldBeNil)
			expectedBlob := []byte(`{"topic":"projects/app/topics/auth-db-changed","authorized":true,"gs":{"auth_db_gs_path":"chrome-infra-auth-test.appspot.com/auth-db","authorized":true}}`)
			So(rw.Body.Bytes(), ShouldEqual, expectedBlob)
		})

		Convey("succeeds for authorized user", func() {
			// Set expected GS client calls for test setup, followed by
			// expected authorization of the user from Subscribe.
			gomock.InOrder(
				mockGSClient.Client.EXPECT().UpdateReadACL(
					gomock.Any(), gomock.Any(), stringset.NewFromSlice("somebody@example.com")).Times(2),
				mockGSClient.Client.EXPECT().Close().Times(1),
				mockGSClient.Client.EXPECT().UpdateReadACL(
					gomock.Any(), gomock.Any(), stringset.NewFromSlice("somebody@example.com")).Times(2),
				mockGSClient.Client.EXPECT().Close().Times(1))

			// Set expected Pubsub client calls.
			gomock.InOrder(
				mockPubsubClient.Client.EXPECT().GetIAMPolicy(gomock.Any()).Return(pubsub.StubPolicy("somebody@example.com"), nil).Times(1),
				mockPubsubClient.Client.EXPECT().Close().Times(1))

			// Set up an authorized user.
			So(model.AuthorizeReader(ctx, "somebody@example.com"), ShouldBeNil)

			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:somebody@example.com",
			})
			rw := httptest.NewRecorder()
			rctx := &router.Context{
				Request: (&http.Request{}).WithContext(ctx),
				Writer:  rw,
			}
			err := Authorize(rctx)
			So(err, ShouldBeNil)
			expectedBlob := []byte(`{"topic":"projects/app/topics/auth-db-changed","authorized":true,"gs":{"auth_db_gs_path":"chrome-infra-auth-test.appspot.com/auth-db","authorized":true}}`)
			So(rw.Body.Bytes(), ShouldEqual, expectedBlob)
		})
	})
}

func TestDeauthorize(t *testing.T) {
	t.Parallel()

	Convey("Deauthorize works", t, func() {
		ctx := memory.Use(context.Background())
		ctx = clock.Set(ctx, testclock.New(testModifiedTS))

		// Set up mock GS client and mock Pubsub client
		ctl := gomock.NewController(t)
		mockGSClient := gs.NewMockedClient(ctx, ctl)
		mockPubsubClient := pubsub.NewMockedClient(mockGSClient.Ctx, ctl)
		ctx = mockPubsubClient.Ctx

		// Set up settings config.
		cfg := &configspb.SettingsCfg{
			AuthDbGsPath: "chrome-infra-auth-test.appspot.com/auth-db",
		}
		So(settingscfg.SetConfig(ctx, cfg), ShouldBeNil)

		Convey("user must use email-based auth", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			rw := httptest.NewRecorder()
			rctx := &router.Context{
				Request: (&http.Request{}).WithContext(ctx),
				Writer:  rw,
			}
			err := Deauthorize(rctx)
			So(err, ShouldErrLike, "error getting caller email")
			So(status.Code(err), ShouldEqual, codes.InvalidArgument)
			So(rw.Body.Bytes(), ShouldBeEmpty)
		})

		Convey("revokes for authorized user", func() {
			// Set expected GS client calls for test setup, followed by
			// expected deauthorization of the user from Unsubscribe.
			gomock.InOrder(
				mockGSClient.Client.EXPECT().UpdateReadACL(
					gomock.Any(), gomock.Any(), stringset.NewFromSlice("someone@example.com")).Times(2),
				mockGSClient.Client.EXPECT().Close().Times(1),
				mockGSClient.Client.EXPECT().UpdateReadACL(
					gomock.Any(), gomock.Any(), stringset.Set{}).Times(2),
				mockGSClient.Client.EXPECT().Close().Times(1))

			// Set expected Pubsub client calls.
			gomock.InOrder(
				mockPubsubClient.Client.EXPECT().GetIAMPolicy(gomock.Any()).Return(pubsub.StubPolicy("someone@example.com"), nil).Times(1),
				mockPubsubClient.Client.EXPECT().SetIAMPolicy(gomock.Any(), gomock.Any()).Times(1),
				mockPubsubClient.Client.EXPECT().Close().Times(1))

			// Set up an authorized user.
			So(model.AuthorizeReader(ctx, "someone@example.com"), ShouldBeNil)

			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
			})
			rw := httptest.NewRecorder()
			rctx := &router.Context{
				Request: (&http.Request{}).WithContext(ctx),
				Writer:  rw,
			}
			err := Deauthorize(rctx)
			So(err, ShouldBeNil)
			expectedBlob := []byte(`{"topic":"projects/app/topics/auth-db-changed","authorized":false,"gs":{"auth_db_gs_path":"chrome-infra-auth-test.appspot.com/auth-db","authorized":false}}`)
			So(rw.Body.Bytes(), ShouldEqual, expectedBlob)
		})

		Convey("succeeds for unauthorized user", func() {
			// Set expected client calls from updating ACLs.
			gomock.InOrder(
				mockGSClient.Client.EXPECT().UpdateReadACL(
					gomock.Any(), gomock.Any(), stringset.Set{}).Times(2),
				mockGSClient.Client.EXPECT().Close().Times(1))

			// Set expected Pubsub client calls.
			gomock.InOrder(
				mockPubsubClient.Client.EXPECT().GetIAMPolicy(gomock.Any()).Return(pubsub.StubPolicy(), nil).Times(1),
				mockPubsubClient.Client.EXPECT().Close().Times(1))

			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:somebody@example.com",
			})
			rw := httptest.NewRecorder()
			rctx := &router.Context{
				Request: (&http.Request{}).WithContext(ctx),
				Writer:  rw,
			}
			err := Deauthorize(rctx)
			So(err, ShouldBeNil)
			expectedBlob := []byte(`{"topic":"projects/app/topics/auth-db-changed","authorized":false,"gs":{"auth_db_gs_path":"chrome-infra-auth-test.appspot.com/auth-db","authorized":false}}`)
			So(rw.Body.Bytes(), ShouldEqual, expectedBlob)
		})
	})
}
