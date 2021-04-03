// Copyright 2021 The LUCI Authors.
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

package config

import (
	"context"
	"testing"

	luciconfig "go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"

	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

const settingsContent = `
resultdb {
  hostname: "testing.results.api.cr.dev"
}
`

// fakeCfgClient mocks the luciconfig.Interface.
type fakeCfgClient struct {
	luciconfig.Interface
}

func (*fakeCfgClient) GetConfig(ctx context.Context, configSet luciconfig.Set, path string, metaOnly bool) (*luciconfig.Config, error) {
	if path == "settings.cfg" {
		return &luciconfig.Config{Content: settingsContent}, nil
	}
	return nil, nil
}

func TestConfig(t *testing.T) {
	t.Parallel()
	Convey("GetSettingsCfg", t, func() {
		ctx := context.Background()
		ctx = cfgclient.Use(ctx, &fakeCfgClient{})
		Convey("success", func() {
			settingsCfg, err := GetSettingsCfg(ctx)
			So(err, ShouldBeNil)
			So(settingsCfg, ShouldResembleProto, &pb.SettingsCfg{Resultdb: &pb.ResultDBSettings{Hostname: "testing.results.api.cr.dev"}})
		})
		Convey("error", func() {
			ctx = context.Background()
			_, err := GetSettingsCfg(ctx)
			So(err, ShouldErrLike, "loading settings.cfg from luci-config")
		})
	})
}
