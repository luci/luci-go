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

func TestParseCL(t *testing.T) {
	Convey("ParseCL", t, func() {

		Convey("https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/1541677/7", func() {
			actual, err := parseCL("https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/1541677/7")
			So(err, ShouldBeNil)
			So(actual, ShouldResembleProto, &pb.GerritChange{
				Host:     "chromium-review.googlesource.com",
				Project:  "infra/luci/luci-go",
				Change:   1541677,
				Patchset: 7,
			})
		})

		Convey("https://chromium-review.googlesource.com/#/c/infra/luci/luci-go/+/1541677/7", func() {
			actual, err := parseCL("https://chromium-review.googlesource.com/#/c/infra/luci/luci-go/+/1541677/7")
			So(err, ShouldBeNil)
			So(actual, ShouldResembleProto, &pb.GerritChange{
				Host:     "chromium-review.googlesource.com",
				Project:  "infra/luci/luci-go",
				Change:   1541677,
				Patchset: 7,
			})
		})

		Convey("https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/1541677/7/buildbucket/cmd/bb/base_command.go", func() {
			actual, err := parseCL("https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/1541677/7/buildbucket/cmd/bb/base_command.go")
			So(err, ShouldBeNil)
			So(actual, ShouldResembleProto, &pb.GerritChange{
				Host:     "chromium-review.googlesource.com",
				Project:  "infra/luci/luci-go",
				Change:   1541677,
				Patchset: 7,
			})
		})

		Convey("https://chromium-review.googlesource.com/c/1541677/7", func() {
			actual, err := parseCL("https://chromium-review.googlesource.com/c/1541677/7")
			So(err, ShouldBeNil)
			So(actual, ShouldResembleProto, &pb.GerritChange{
				Host:     "chromium-review.googlesource.com",
				Change:   1541677,
				Patchset: 7,
			})
		})

		Convey("https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/1541677", func() {
			actual, err := parseCL("https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/1541677")
			So(err, ShouldBeNil)
			So(actual, ShouldResembleProto, &pb.GerritChange{
				Host:    "chromium-review.googlesource.com",
				Project: "infra/luci/luci-go",
				Change:  1541677,
			})
		})

		Convey("crrev.com/c/123", func() {
			actual, err := parseCL("crrev.com/c/123")
			So(err, ShouldBeNil)
			So(actual, ShouldResembleProto, &pb.GerritChange{
				Host:   "chromium-review.googlesource.com",
				Change: 123,
			})
		})

		Convey("crrev.com/c/123/4", func() {
			actual, err := parseCL("crrev.com/c/123/4")
			So(err, ShouldBeNil)
			So(actual, ShouldResembleProto, &pb.GerritChange{
				Host:     "chromium-review.googlesource.com",
				Change:   123,
				Patchset: 4,
			})
		})

		Convey("crrev.com/i/123", func() {
			actual, err := parseCL("crrev.com/i/123")
			So(err, ShouldBeNil)
			So(actual, ShouldResembleProto, &pb.GerritChange{
				Host:   "chrome-internal-review.googlesource.com",
				Change: 123,
			})
		})

		Convey("https://crrev.com/i/123", func() {
			actual, err := parseCL("https://crrev.com/i/123")
			So(err, ShouldBeNil)
			So(actual, ShouldResembleProto, &pb.GerritChange{
				Host:   "chrome-internal-review.googlesource.com",
				Change: 123,
			})
		})

		Convey("https://chrome-internal-review.googlesource.com/c/src/+/1/2", func() {
			actual, err := parseCL("https://chrome-internal-review.googlesource.com/c/src/+/1/2")
			So(err, ShouldBeNil)
			So(actual, ShouldResembleProto, &pb.GerritChange{
				Host:     "chrome-internal-review.googlesource.com",
				Project:  "src",
				Change:   1,
				Patchset: 2,
			})
		})
	})
}
