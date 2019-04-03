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

package cli

import (
	"testing"

	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestStatusFlag(t *testing.T) {
	t.Parallel()

	Convey("StatusFlag", t, func() {
		var status pb.Status
		f := StatusFlag(&status)

		Convey("Get", func() {
			So(f.Get(), ShouldEqual, status)
			status = pb.Status_SUCCESS
			So(f.Get(), ShouldEqual, status)
		})

		Convey("String", func() {
			So(f.String(), ShouldEqual, "")
			status = pb.Status_SUCCESS
			So(f.String(), ShouldEqual, "success")
		})

		Convey("Set", func() {
			Convey("success", func() {
				So(f.Set("success"), ShouldBeNil)
				So(status, ShouldEqual, pb.Status_SUCCESS)
			})
			Convey("SUCCESS", func() {
				So(f.Set("SUCCESS"), ShouldBeNil)
				So(status, ShouldEqual, pb.Status_SUCCESS)
			})
			Convey("unspecified", func() {
				So(f.Set("unspecified"), ShouldErrLike, `invalid status "unspecified"; expected one of canceled, ended, failure, infra_faiure, scheduled, started, success`)
			})
			Convey("garbage", func() {
				So(f.Set("garbage"), ShouldErrLike, `invalid status "garbage"; expected one of canceled, ended, failure, infra_faiure, scheduled, started, success`)
			})
		})
	})
}
