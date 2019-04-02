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

package protoutil

import (
	"testing"

	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestParseGetBuildRequest(t *testing.T) {
	t.Parallel()

	Convey("ParseGetBuildRequest", t, func() {
		Convey("build id", func() {
			req, err := ParseGetBuildRequest("1234567890")
			So(err, ShouldBeNil)
			So(req, ShouldResembleProto, &pb.GetBuildRequest{Id: 1234567890})
		})

		Convey("build number", func() {
			req, err := ParseGetBuildRequest("chromium/ci/linux-rel/1")
			So(err, ShouldBeNil)
			So(req, ShouldResembleProtoText, `
				builder {
					project: "chromium"
					bucket: "ci"
					builder: "linux-rel"
				}
				build_number: 1
			`)
		})
	})
}
