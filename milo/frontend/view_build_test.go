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

package frontend

import (
	"testing"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPrepareGetBuildRequest(t *testing.T) {
	t.Parallel()

	Convey(`TestPrepareGetBuildRequest`, t, func() {
		builderID := &buildbucketpb.BuilderID{
			Project: "fake-project",
			Bucket:  "fake-bucket",
			Builder: "fake-builder",
		}

		Convey("Should parse build number correctly", func() {
			req, err := prepareGetBuildRequest(builderID, "123")
			So(err, ShouldBeNil)
			So(req.BuildNumber, ShouldEqual, 123)
			So(req.Builder, ShouldEqual, builderID)
		})

		Convey("Should reject large build number", func() {
			req, err := prepareGetBuildRequest(builderID, "9223372036854775807")
			So(err, ShouldNotBeNil)
			So(req, ShouldBeNil)
		})

		Convey("Should reject malformated build number", func() {
			req, err := prepareGetBuildRequest(builderID, "abc")
			So(err, ShouldNotBeNil)
			So(req, ShouldBeNil)
		})

		Convey("Should parse build ID correctly", func() {
			req, err := prepareGetBuildRequest(builderID, "b123")
			So(err, ShouldBeNil)
			So(req.Id, ShouldEqual, 123)
			So(req.Builder, ShouldBeNil)
		})

		Convey("Should reject large build ID", func() {
			req, err := prepareGetBuildRequest(builderID, "b9223372036854775809")
			So(err, ShouldNotBeNil)
			So(req, ShouldBeNil)
		})
	})
}
