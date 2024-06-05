// Copyright 2022 The LUCI Authors.
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

package oauthcfg

import (
	"context"
	"testing"

	"go.chromium.org/luci/gae/impl/memory"

	"go.chromium.org/luci/auth_service/api/configspb"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestConfigContext(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	oauthCfg := &configspb.OAuthConfig{
		PrimaryClientId:     "424242.test.example.com",
		PrimaryClientSecret: "SuPeRsecRetSTring",
		ClientIds: []string{
			"123456789.test.example.com",
			"987654321.test.example.com",
		},
		TokenServerUrl: "https://test-token-server.example.com",
	}

	Convey("Getting without setting fails", t, func() {
		_, err := Get(ctx)
		So(err, ShouldNotBeNil)
	})

	Convey("Testing basic Config operations", t, func() {
		So(SetConfig(ctx, oauthCfg), ShouldBeNil)
		cfgFromGet, err := Get(ctx)
		So(err, ShouldBeNil)
		So(cfgFromGet, ShouldResembleProto, oauthCfg)
	})
}
