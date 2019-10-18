// Copyright 2019 The LUCI Authors.
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

package monitoring

import (
	"context"
	"testing"

	gae "go.chromium.org/gae/impl/memory"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/impl/memory"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestFetch(t *testing.T) {
	t.Parallel()
	ctx := gae.UseWithAppID(context.Background(), "cipd")

	Convey("fetch", t, func() {
		Convey("invalid", func() {
			cli := memory.New(map[config.Set]memory.Files{
				"services/cipd": map[string]string{
					cfgFile: "invalid",
				},
			})
			So(importConfig(ctx, cli), ShouldErrLike, "failed to parse")
		})

		Convey("empty", func() {
			cli := memory.New(map[config.Set]memory.Files{
				"services/cipd": map[string]string{
					cfgFile: `
					client_monitoring_config <
						ip_whitelist: "whitelist-1"
						label: "label-1"
					>
					client_monitoring_config <
						ip_whitelist: "whitelist-2"
						label: "label-2"
					>
					`,
				},
			})
			So(importConfig(ctx, cli), ShouldBeNil)
		})
	})
}
