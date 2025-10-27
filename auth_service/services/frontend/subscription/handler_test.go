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
	"encoding/json"
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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/auth_service/api/configspb"
	"go.chromium.org/luci/auth_service/impl/model"
	"go.chromium.org/luci/auth_service/internal/configs/srvcfg/settingscfg"
	"go.chromium.org/luci/auth_service/internal/gs"
	"go.chromium.org/luci/auth_service/internal/pubsub"
)

var (
	testModifiedTS = time.Date(2021, time.August, 16, 12, 20, 0, 0, time.UTC)
)

func TestCheckAccess(t *testing.T) {
	t.Parallel()

	ftt.Run("CheckAccess works", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx = clock.Set(ctx, testclock.New(testModifiedTS))

		// Set up mock Pubsub client
		ctl := gomock.NewController(t)
		mockPubsubClient := pubsub.NewMockedClient(ctx, ctl)
		ctx = mockPubsubClient.Ctx
		policy := pubsub.StubPolicy("user:someone@example.com")

		// Set up settings config.
		cfg := &configspb.SettingsCfg{}
		assert.Loosely(t, settingscfg.SetInTest(ctx, cfg, nil), should.BeNil)

		// Set up an authorized user.
		assert.Loosely(t, model.AuthorizeReader(ctx, "someone@example.com"), should.BeNil)

		t.Run("user must use email-based auth", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			rw := httptest.NewRecorder()
			rctx := &router.Context{
				Request: (&http.Request{}).WithContext(ctx),
				Writer:  rw,
			}
			err := CheckAccess(rctx)
			assert.Loosely(t, err, should.ErrLike("error getting caller email"))
			assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, rw.Body.Bytes(), should.BeEmpty)
		})

		t.Run("false for unauthorized", func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)

			actual := map[string]any{}
			assert.Loosely(t, json.NewDecoder(rw.Body).Decode(&actual), should.BeNil)
			assert.Loosely(t, actual, should.Match(map[string]any{
				"topic":      "projects/app/topics/auth-db-changed",
				"authorized": false,
				"gs": map[string]any{
					"auth_db_gs_path": "",
					"authorized":      false,
				},
			}))
		})

		t.Run("true for authorized", func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)

			actual := map[string]any{}
			assert.Loosely(t, json.NewDecoder(rw.Body).Decode(&actual), should.BeNil)
			assert.Loosely(t, actual, should.Match(map[string]any{
				"topic":      "projects/app/topics/auth-db-changed",
				"authorized": true,
				"gs": map[string]any{
					"auth_db_gs_path": "",
					"authorized":      true,
				},
			}))
		})
	})
}

func TestAuthorize(t *testing.T) {
	t.Parallel()

	ftt.Run("Authorize works", t, func(t *ftt.Test) {
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
		assert.Loosely(t, settingscfg.SetInTest(ctx, cfg, nil), should.BeNil)

		t.Run("user must use email-based auth", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			rw := httptest.NewRecorder()
			rctx := &router.Context{
				Request: (&http.Request{}).WithContext(ctx),
				Writer:  rw,
			}
			err := Authorize(rctx)
			assert.Loosely(t, err, should.ErrLike("error getting caller email"))
			assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, rw.Body.Bytes(), should.BeEmpty)
		})

		t.Run("denies ineligible user", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
			})
			rw := httptest.NewRecorder()
			rctx := &router.Context{
				Request: (&http.Request{}).WithContext(ctx),
				Writer:  rw,
			}
			err := Authorize(rctx)
			assert.Loosely(t, err, should.ErrLike("ineligible to subscribe"))
			assert.Loosely(t, status.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, rw.Body.Bytes(), should.BeEmpty)
		})

		t.Run("authorizes a new user", func(t *ftt.Test) {
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
				Identity:       "user:someone@example.com",
				IdentityGroups: []string{model.TrustedServicesGroup},
			})
			rw := httptest.NewRecorder()
			rctx := &router.Context{
				Request: (&http.Request{}).WithContext(ctx),
				Writer:  rw,
			}
			err := Authorize(rctx)
			assert.Loosely(t, err, should.BeNil)

			actual := map[string]any{}
			assert.Loosely(t, json.NewDecoder(rw.Body).Decode(&actual), should.BeNil)
			assert.Loosely(t, actual, should.Match(map[string]any{
				"topic":      "projects/app/topics/auth-db-changed",
				"authorized": true,
				"gs": map[string]any{
					"auth_db_gs_path": "chrome-infra-auth-test.appspot.com/auth-db",
					"authorized":      true,
				},
			}))
		})

		t.Run("succeeds for authorized user", func(t *ftt.Test) {
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
				mockPubsubClient.Client.EXPECT().GetIAMPolicy(gomock.Any()).Return(pubsub.StubPolicy("user:somebody@example.com"), nil).Times(1),
				mockPubsubClient.Client.EXPECT().Close().Times(1))

			// Set up an authorized user.
			assert.Loosely(t, model.AuthorizeReader(ctx, "somebody@example.com"), should.BeNil)

			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:somebody@example.com",
				IdentityGroups: []string{model.TrustedServicesGroup},
			})
			rw := httptest.NewRecorder()
			rctx := &router.Context{
				Request: (&http.Request{}).WithContext(ctx),
				Writer:  rw,
			}
			err := Authorize(rctx)
			assert.Loosely(t, err, should.BeNil)

			actual := map[string]any{}
			assert.Loosely(t, json.NewDecoder(rw.Body).Decode(&actual), should.BeNil)
			assert.Loosely(t, actual, should.Match(map[string]any{
				"topic":      "projects/app/topics/auth-db-changed",
				"authorized": true,
				"gs": map[string]any{
					"auth_db_gs_path": "chrome-infra-auth-test.appspot.com/auth-db",
					"authorized":      true,
				},
			}))
		})
	})
}

func TestDeauthorize(t *testing.T) {
	t.Parallel()

	ftt.Run("Deauthorize works", t, func(t *ftt.Test) {
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
		assert.Loosely(t, settingscfg.SetInTest(ctx, cfg, nil), should.BeNil)

		t.Run("user must use email-based auth", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			rw := httptest.NewRecorder()
			rctx := &router.Context{
				Request: (&http.Request{}).WithContext(ctx),
				Writer:  rw,
			}
			err := Deauthorize(rctx)
			assert.Loosely(t, err, should.ErrLike("error getting caller email"))
			assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, rw.Body.Bytes(), should.BeEmpty)
		})

		t.Run("revokes for authorized user", func(t *ftt.Test) {
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
				mockPubsubClient.Client.EXPECT().GetIAMPolicy(gomock.Any()).Return(pubsub.StubPolicy("user:someone@example.com"), nil).Times(1),
				mockPubsubClient.Client.EXPECT().SetIAMPolicy(gomock.Any(), gomock.Any()).Times(1),
				mockPubsubClient.Client.EXPECT().Close().Times(1))

			// Set up an authorized user.
			assert.Loosely(t, model.AuthorizeReader(ctx, "someone@example.com"), should.BeNil)

			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
			})
			rw := httptest.NewRecorder()
			rctx := &router.Context{
				Request: (&http.Request{}).WithContext(ctx),
				Writer:  rw,
			}
			err := Deauthorize(rctx)
			assert.Loosely(t, err, should.BeNil)

			actual := map[string]any{}
			assert.Loosely(t, json.NewDecoder(rw.Body).Decode(&actual), should.BeNil)
			assert.Loosely(t, actual, should.Match(map[string]any{
				"topic":      "projects/app/topics/auth-db-changed",
				"authorized": false,
				"gs": map[string]any{
					"auth_db_gs_path": "chrome-infra-auth-test.appspot.com/auth-db",
					"authorized":      false,
				},
			}))
		})

		t.Run("succeeds for unauthorized user", func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)

			actual := map[string]any{}
			assert.Loosely(t, json.NewDecoder(rw.Body).Decode(&actual), should.BeNil)
			assert.Loosely(t, actual, should.Match(map[string]any{
				"topic":      "projects/app/topics/auth-db-changed",
				"authorized": false,
				"gs": map[string]any{
					"auth_db_gs_path": "chrome-infra-auth-test.appspot.com/auth-db",
					"authorized":      false,
				},
			}))
		})
	})
}
