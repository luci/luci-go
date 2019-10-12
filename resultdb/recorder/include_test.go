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

package main

import (
	"testing"

	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateIncludeRequest(t *testing.T) {
	Convey(`TestValidateIncludeRequest`, t, func() {
		Convey(`Valid`, func() {
			err := validateIncludeRequest(&pb.IncludeRequest{
				IncludingInvocation: "invocations/a",
				IncludedInvocation:  "invocations/b",
				OverrideInvocation:  "invocations/c",
			})
			So(err, ShouldBeNil)
		})

		Convey(`Invalid including_invocation`, func() {
			err := validateIncludeRequest(&pb.IncludeRequest{
				IncludingInvocation: "x",
				IncludedInvocation:  "invocations/c",
			})
			So(err, ShouldErrLike, `including_invocation: does not match`)
		})

		Convey(`Invalid included_invocation`, func() {
			err := validateIncludeRequest(&pb.IncludeRequest{
				IncludingInvocation: "invocations/a",
				IncludedInvocation:  "x",
			})
			So(err, ShouldErrLike, `included_invocation: does not match`)
		})

		Convey(`include itself`, func() {
			err := validateIncludeRequest(&pb.IncludeRequest{
				IncludingInvocation: "invocations/a",
				IncludedInvocation:  "invocations/a",
			})
			So(err, ShouldErrLike, `cannot include itself`)
		})

		Convey(`Invalid override_invocation`, func() {
			err := validateIncludeRequest(&pb.IncludeRequest{
				IncludingInvocation: "invocations/a",
				IncludedInvocation:  "invocations/b",
				OverrideInvocation:  "x",
			})
			So(err, ShouldErrLike, `override_invocation: does not match`)
		})

		Convey(`override itself`, func() {
			err := validateIncludeRequest(&pb.IncludeRequest{
				IncludingInvocation: "invocations/a",
				IncludedInvocation:  "invocations/b",
				OverrideInvocation:  "invocations/b",
			})
			So(err, ShouldErrLike, `cannot override itself`)
		})

		Convey(`include itself via override`, func() {
			err := validateIncludeRequest(&pb.IncludeRequest{
				IncludingInvocation: "invocations/a",
				IncludedInvocation:  "invocations/b",
				OverrideInvocation:  "invocations/a",
			})
			So(err, ShouldErrLike, `cannot include itself`)
		})
	})
}
