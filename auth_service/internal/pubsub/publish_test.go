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

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/service/protocol"
	"go.chromium.org/luci/server/auth/signing"
	"go.chromium.org/luci/server/auth/signing/signingtest"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPublishAuthDBRevision(t *testing.T) {
	t.Parallel()

	Convey("PublishAuthDBRevision works", t, func() {
		testAppID := "chrome-infra-auth-test"
		ctx := memory.UseWithAppID(context.Background(), "dev~"+testAppID)
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
		err := PublishAuthDBRevision(ctx, testRev, false)
		So(err, ShouldBeNil)
	})
}
