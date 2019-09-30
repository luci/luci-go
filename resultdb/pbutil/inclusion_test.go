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

package pbutil

import (
	"testing"

	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestInclusionName(t *testing.T) {
	Convey("ParseInclusionName", t, func() {
		Convey("Parse", func() {
			including, included, err := ParseInclusionName("invocations/a/inclusions/b")
			So(err, ShouldBeNil)
			So(including, ShouldEqual, "a")
			So(included, ShouldEqual, "b")
		})

		Convey("Invalid name", func() {
			_, _, err := ParseInclusionName("invocations/a/inclusions")
			So(err, ShouldErrLike, `does not match`)
		})

		Convey("Format", func() {
			So(InclusionName("a", "b"), ShouldEqual, "invocations/a/inclusions/b")
		})
	})
}

func TestValidateCreateInclusionRequest(t *testing.T) {
	Convey(`ValidateCreateInclusionRequest`, t, func() {
		Convey(`Valid`, func() {
			err := ValidateCreateInclusionRequest(&pb.CreateInclusionRequest{
				IncludingInvocation: "invocations/a",
				Inclusion: &pb.Inclusion{
					IncludedInvocation: "invocations/b",
				},
			})
			So(err, ShouldBeNil)
		})

		Convey(`Invalid including_invocation`, func() {
			err := ValidateCreateInclusionRequest(&pb.CreateInclusionRequest{
				IncludingInvocation: "x",
				Inclusion: &pb.Inclusion{
					IncludedInvocation: "invocations/b",
				},
			})
			So(err, ShouldErrLike, `including_invocation: does not match`)
		})

		Convey(`Invalid included_invocation`, func() {
			err := ValidateCreateInclusionRequest(&pb.CreateInclusionRequest{
				IncludingInvocation: "invocations/a",
				Inclusion: &pb.Inclusion{
					IncludedInvocation: "x",
				},
			})
			So(err, ShouldErrLike, `inclusion.included_invocation: does not match`)
		})

		Convey(`include itself`, func() {
			err := ValidateCreateInclusionRequest(&pb.CreateInclusionRequest{
				IncludingInvocation: "invocations/a",
				Inclusion: &pb.Inclusion{
					IncludedInvocation: "invocations/a",
				},
			})
			So(err, ShouldErrLike, `cannot include itself`)
		})
	})
}

func TestValidateOverrideInclusionRequest(t *testing.T) {
	Convey(`ValidateOverrideInclusionRequest`, t, func() {
		Convey(`Valid`, func() {
			err := ValidateOverrideInclusionRequest(&pb.OverrideInclusionRequest{
				IncludingInvocation:          "invocations/a",
				OverriddenIncludedInvocation: "invocations/b",
				OverridingIncludedInvocation: "invocations/c",
			})
			So(err, ShouldBeNil)
		})

		Convey(`Invalid including_invocation`, func() {
			err := ValidateOverrideInclusionRequest(&pb.OverrideInclusionRequest{
				IncludingInvocation:          "x",
				OverriddenIncludedInvocation: "invocations/b",
				OverridingIncludedInvocation: "invocations/c",
			})
			So(err, ShouldErrLike, `including_invocation: does not match`)
		})

		Convey(`Invalid overridden_included_invocation`, func() {
			err := ValidateOverrideInclusionRequest(&pb.OverrideInclusionRequest{
				IncludingInvocation:          "invocations/a",
				OverriddenIncludedInvocation: "x",
				OverridingIncludedInvocation: "invocations/c",
			})
			So(err, ShouldErrLike, `overridden_included_invocation: does not match`)
		})

		Convey(`Invalid overriding_included_invocation`, func() {
			err := ValidateOverrideInclusionRequest(&pb.OverrideInclusionRequest{
				IncludingInvocation:          "invocations/a",
				OverriddenIncludedInvocation: "invocations/b",
				OverridingIncludedInvocation: "x",
			})
			So(err, ShouldErrLike, `overriding_included_invocation: does not match`)
		})

		Convey(`include itself`, func() {
			err := ValidateOverrideInclusionRequest(&pb.OverrideInclusionRequest{
				IncludingInvocation:          "invocations/a",
				OverriddenIncludedInvocation: "invocations/b",
				OverridingIncludedInvocation: "invocations/a",
			})
			So(err, ShouldErrLike, `cannot include itself`)
		})

		Convey(`override itself`, func() {
			err := ValidateOverrideInclusionRequest(&pb.OverrideInclusionRequest{
				IncludingInvocation:          "invocations/a",
				OverriddenIncludedInvocation: "invocations/b",
				OverridingIncludedInvocation: "invocations/b",
			})
			So(err, ShouldErrLike, `cannot override itself`)
		})
	})
}
