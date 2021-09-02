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

	"go.chromium.org/luci/gae/impl/memory"

	// TODO(crbug/1242998): Remove once safe get becomes datastore default.
	_ "go.chromium.org/luci/gae/service/datastore/crbug1242998safeget"

	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestConfig(t *testing.T) {
	t.Parallel()
	Convey("get settings.cfg", t, func() {
		settingsCfg := &pb.SettingsCfg{Resultdb: &pb.ResultDBSettings{Hostname: "testing.results.api.cr.dev"}}
		ctx := memory.Use(context.Background())
		SetTestSettingsCfg(ctx, settingsCfg)
		cfg, err := GetSettingsCfg(ctx)
		So(err, ShouldBeNil)
		So(cfg, ShouldResembleProto, settingsCfg)
	})
}
