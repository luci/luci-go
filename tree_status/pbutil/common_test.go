// Copyright 2024 The LUCI Authors.
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

package pbutil

import (
	"strings"
	"testing"

	pb "go.chromium.org/luci/tree_status/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidation(t *testing.T) {
	Convey("tree Name", t, func() {
		Convey("tree Name", func() {
			Convey("valid", func() {
				err := ValidateTreeName("fuchsia-stem")
				So(err, ShouldBeNil)
			})
			Convey("must be specified", func() {
				err := ValidateTreeName("")
				So(err, ShouldErrLike, "must be specified")
			})
			Convey("must match format", func() {
				err := ValidateTreeName("INVALID")
				So(err, ShouldErrLike, "expected format")
			})
		})

		Convey("status ID", func() {
			Convey("valid", func() {
				err := ValidateStatusID(strings.Repeat("0", 32))
				So(err, ShouldBeNil)
			})
			Convey("must be specified", func() {
				err := ValidateStatusID("")
				So(err, ShouldErrLike, "must be specified")
			})
			Convey("must match format", func() {
				err := ValidateStatusID("INVALID")
				So(err, ShouldErrLike, "expected format")
			})
		})

		Convey("general status", func() {
			Convey("valid", func() {
				err := ValidateGeneralStatus(pb.GeneralState_CLOSED)
				So(err, ShouldBeNil)
			})
			Convey("must be specified", func() {
				err := ValidateGeneralStatus(pb.GeneralState_GENERAL_STATE_UNSPECIFIED)
				So(err, ShouldErrLike, "must be specified")
			})
			Convey("must match format", func() {
				err := ValidateGeneralStatus(pb.GeneralState(100))
				So(err, ShouldErrLike, "invalid enum value")
			})
		})

		Convey("message", func() {
			Convey("valid", func() {
				err := ValidateMessage("my message")
				So(err, ShouldBeNil)
			})
			Convey("must be specified", func() {
				err := ValidateMessage("")
				So(err, ShouldErrLike, "must be specified")
			})
			Convey("must not exceed length", func() {
				err := ValidateMessage(strings.Repeat("a", 1025))
				So(err, ShouldErrLike, "longer than 1024 bytes")
			})
			Convey("invalid utf-8 string", func() {
				err := ValidateMessage("\xbd")
				So(err, ShouldErrLike, "not a valid utf8 string")
			})
			Convey("not in unicode normalized form C", func() {
				err := ValidateMessage("\u0065\u0301")
				So(err, ShouldErrLike, "not in unicode normalized form C")
			})
			Convey("non printable rune", func() {
				err := ValidateMessage("non printable rune\u0007")
				So(err, ShouldErrLike, "non-printable rune")
			})
		})

		Convey("closing builder name", func() {
			Convey("valid", func() {
				err := ValidateClosingBuilderName("projects/chromium-m100/buckets/ci.shadow/builders/Linux 123")
				So(err, ShouldBeNil)
			})
			Convey("empty closing builder is OK", func() {
				err := ValidateClosingBuilderName("")
				So(err, ShouldBeNil)
			})
			Convey("must match format", func() {
				err := ValidateClosingBuilderName("some name")
				So(err, ShouldErrLike, "expected format")
			})
		})
	})
}
