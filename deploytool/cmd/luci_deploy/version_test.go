// Copyright 2016 The LUCI Authors.
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

package main

import (
	"testing"

	"go.chromium.org/luci/deploytool/api/deploy"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCloudProjectVersion(t *testing.T) {
	Convey(`A cloud project version`, t, func() {
		b := cloudProjectVersionBuilder{
			currentUser: func() (string, error) {
				return "test-person", nil
			},
		}
		cp := layoutDeploymentCloudProject{
			Deployment_CloudProject: &deploy.Deployment_CloudProject{
				VersionScheme: deploy.Deployment_CloudProject_DEFAULT,
			},
		}
		src := layoutSource{
			FrozenLayout_Source: &deploy.FrozenLayout_Source{
				MinorVersion: "--minor",
				MajorVersion: "major",
			},
			sg: &layoutSourceGroup{
				FrozenLayout_SourceGroup: &deploy.FrozenLayout_SourceGroup{},
			},
		}

		Convey(`Renders and parses without a tainted user.`, func() {
			v, err := b.build(&cp, &src)
			So(err, ShouldBeNil)

			s := v.String()
			So(s, ShouldEqual, "__minor-major")

			parsed, err := parseCloudProjectVersion(deploy.Deployment_CloudProject_DEFAULT, s)
			So(err, ShouldBeNil)
			So(parsed, ShouldResemble, v)
		})

		Convey(`Renders and parses with a tainted user.`, func() {
			src.sg.Tainted = true

			v, err := b.build(&cp, &src)
			So(err, ShouldBeNil)

			s := v.String()
			So(s, ShouldEqual, "__minor-major-tainted-test_person")

			parsed, err := parseCloudProjectVersion(deploy.Deployment_CloudProject_DEFAULT, s)
			So(err, ShouldBeNil)
			So(parsed, ShouldResemble, v)
		})
	})
}
