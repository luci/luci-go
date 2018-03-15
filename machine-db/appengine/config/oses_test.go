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
	"testing"

	"golang.org/x/net/context"

	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/machine-db/api/config/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateOSes(t *testing.T) {
	t.Parallel()

	Convey("validateOSes", t, func() {
		context := &validation.Context{Context: context.Background()}

		Convey("empty config", func() {
			oses := &config.OSes{}
			validateOSes(context, oses)
			So(context.Finalize(), ShouldBeNil)
		})

		Convey("unnamed operating system", func() {
			oses := &config.OSes{
				OperatingSystem: []*config.OS{
					{
						Name: "",
					},
				},
			}
			validateOSes(context, oses)
			So(context.Finalize(), ShouldErrLike, "operating system names are required and must be non-empty")
		})

		Convey("duplicate operating system", func() {
			oses := &config.OSes{
				OperatingSystem: []*config.OS{
					{
						Name: "duplicate",
					},
					{
						Name: "os",
					},
					{
						Name: "duplicate",
					},
				},
			}
			validateOSes(context, oses)
			So(context.Finalize(), ShouldErrLike, "duplicate operating system")
		})

		Convey("ok", func() {
			oses := &config.OSes{
				OperatingSystem: []*config.OS{
					{
						Name: "os 1",
					},
					{
						Name: "os 2",
					},
				},
			}
			validateOSes(context, oses)
			So(context.Finalize(), ShouldBeNil)
		})
	})
}
