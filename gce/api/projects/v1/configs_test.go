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

package projects

import (
	"context"
	"testing"

	"go.chromium.org/luci/config/validation"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestConfigs(t *testing.T) {
	t.Parallel()

	Convey("validate", t, func() {
		c := &validation.Context{Context: context.Background()}

		Convey("invalid", func() {
			Convey("missing", func() {
				cfgs := &Configs{
					Project: []*Config{
						{},
					},
				}
				cfgs.Validate(c)
				errs := c.Finalize().(*validation.Error).Errors
				So(errs, ShouldContainErr, "project is required")
			})

			Convey("duplicate", func() {
				cfgs := &Configs{
					Project: []*Config{
						{
							Project: "duplicated",
						},
						{
							Project: "unique",
						},
						{
							Project: "duplicated",
						},
					},
				}
				cfgs.Validate(c)
				errs := c.Finalize().(*validation.Error).Errors
				So(errs, ShouldContainErr, "is not unique")
			})
		})

		Convey("valid", func() {
			Convey("empty", func() {
				cfgs := &Configs{}
				cfgs.Validate(c)
				So(c.Finalize(), ShouldBeNil)
			})

			Convey("project", func() {
				cfgs := &Configs{
					Project: []*Config{
						{
							Project: "project",
						},
					},
				}
				cfgs.Validate(c)
				So(c.Finalize(), ShouldBeNil)
			})
		})
	})
}
