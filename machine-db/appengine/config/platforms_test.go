// Copyright 2017 The LUCI Authors.
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

	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/machine-db/api/config/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidatePlatforms(t *testing.T) {
	t.Parallel()

	Convey("validatePlatforms", t, func() {
		context := &validation.Context{Context: context.Background()}

		Convey("empty config", func() {
			platforms := &config.Platforms{}
			validatePlatforms(context, platforms)
			So(context.Finalize(), ShouldBeNil)
		})

		Convey("unnamed platform", func() {
			platforms := &config.Platforms{
				Platform: []*config.Platform{
					{
						Name: "",
					},
				},
			}
			validatePlatforms(context, platforms)
			So(context.Finalize(), ShouldErrLike, "platform names are required and must be non-empty")
		})

		Convey("duplicate platform", func() {
			platforms := &config.Platforms{
				Platform: []*config.Platform{
					{
						Name: "duplicate",
					},
					{
						Name: "platform",
					},
					{
						Name: "duplicate",
					},
				},
			}
			validatePlatforms(context, platforms)
			So(context.Finalize(), ShouldErrLike, "duplicate platform")
		})

		Convey("ok", func() {
			platforms := &config.Platforms{
				Platform: []*config.Platform{
					{
						Name: "platform 1",
					},
					{
						Name: "platform 2",
					},
				},
			}
			validatePlatforms(context, platforms)
			So(context.Finalize(), ShouldBeNil)
		})
	})
}
