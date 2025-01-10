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
	"time"

	"github.com/golang/mock/gomock"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/service/protocol"
	"go.chromium.org/luci/server/auth/signing"
	"go.chromium.org/luci/server/auth/signing/signingtest"
)

func TestPublishAuthDBRevision(t *testing.T) {
	t.Parallel()

	testAppID := "chrome-infra-auth-test"
	ftt.Run("PublishAuthDBRevision works", t, func(t *ftt.Test) {

		t.Run("returns error for invalid revision", func(t *ftt.Test) {
			ctx := context.Background()
			assert.Loosely(t, PublishAuthDBRevision(ctx, nil), should.ErrLike("invalid AuthDBRevision"))
		})

		t.Run("skips publishing for local dev server", func(t *ftt.Test) {
			ctx := memory.UseWithAppID(context.Background(), "dev~"+testAppID)

			testModifiedTS := time.Date(2021, time.August, 16, 12, 20, 0, 0, time.UTC)
			testRev := &protocol.AuthDBRevision{
				PrimaryId:  info.AppID(ctx),
				AuthDbRev:  123,
				ModifiedTs: testModifiedTS.UnixMicro(),
			}
			err := PublishAuthDBRevision(ctx, testRev)
			assert.Loosely(t, err, should.BeNil)
		})
	})

	ftt.Run("publish works", t, func(t *ftt.Test) {
		ctx := memory.UseWithAppID(context.Background(), testAppID)
		ctx = auth.ModifyConfig(ctx, func(cfg auth.Config) auth.Config {
			cfg.Signer = signingtest.NewSigner(&signing.ServiceInfo{
				AppID:              testAppID,
				ServiceAccountName: "chrome-infra-auth-test@fake.serviceaccount.com",
			})
			return cfg
		})

		// Set up mock Pubsub client
		ctl := gomock.NewController(t)
		mockClient := NewMockedClient(ctx, ctl)
		ctx = mockClient.Ctx

		// Define expected client calls.
		gomock.InOrder(
			mockClient.Client.EXPECT().Publish(gomock.Any(), gomock.Any()).Times(1),
			mockClient.Client.EXPECT().Close().Times(1),
		)

		testModifiedTS := time.Date(2021, time.August, 16, 12, 20, 0, 0, time.UTC)
		testRev := &protocol.AuthDBRevision{
			PrimaryId:  info.AppID(ctx),
			AuthDbRev:  123,
			ModifiedTs: testModifiedTS.UnixMicro(),
		}
		err := publish(ctx, testRev)
		assert.Loosely(t, err, should.BeNil)
	})
}
