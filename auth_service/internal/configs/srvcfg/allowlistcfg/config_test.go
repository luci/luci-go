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

package allowlistcfg

import (
	"context"
	"testing"

	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/impl/memory"

	"go.chromium.org/luci/auth_service/api/configspb"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestConfigContext(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	allowlistCfg := &configspb.IPAllowlistConfig{
		IpAllowlists: []*configspb.IPAllowlistConfig_IPAllowlist{
			{
				Name: "test-allowlist-1",
				Subnets: []string{
					"127.0.0.1/32",
					"0.0.0.0/0",
				},
			},
			{
				Name: "test-allowlist-2",
				Includes: []string{
					"test-allowlist-1",
				},
			},
		},
		Assignments: []*configspb.IPAllowlistConfig_Assignment{
			{
				Identity:        "abc@example.com",
				IpAllowlistName: "test-allowlist-1",
			},
		},
	}

	Convey("Getting without setting fails", t, func() {
		_, err := Get(ctx)
		So(err, ShouldNotBeNil)

		_, err = GetMetadata(ctx)
		So(err, ShouldNotBeNil)
	})

	Convey("Testing basic Config operations", t, func() {
		So(SetConfig(ctx, allowlistCfg), ShouldBeNil)
		cfgFromGet, err := Get(ctx)
		So(err, ShouldBeNil)
		So(cfgFromGet, ShouldResembleProto, allowlistCfg)
	})

	Convey("Testing config operations with metadata", t, func() {
		metadata := &config.Meta{
			Path:     "ip_allowlist.cfg",
			Revision: "123abc",
			ViewURL:  "https://example.com/config/revision/123abc",
		}
		So(SetConfigWithMetadata(ctx, allowlistCfg, metadata), ShouldBeNil)
		metadataFromGet, err := GetMetadata(ctx)
		So(err, ShouldBeNil)
		So(metadataFromGet, ShouldResembleProto, metadata)
	})
}
