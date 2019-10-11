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

func TestValidateIncludeInvocationRequest(t *testing.T) {
	Convey(`ValidateIncludeInvocationRequest`, t, func() {
		Convey(`Valid`, func() {
			err := ValidateIncludeInvocationRequest(&pb.IncludeInvocationRequest{
				Including: "invocations/a",
				Included:  "invocations/b",
			})
			So(err, ShouldBeNil)
		})

		Convey(`Invalid including_invocation`, func() {
			err := ValidateIncludeInvocationRequest(&pb.IncludeInvocationRequest{
				Including: "x",
				Included:  "invocations/c",
				Overrides: []string{"invocations/b"},
			})
			So(err, ShouldErrLike, `including_invocation: does not match`)
		})

		Convey(`Invalid overridden_included_invocation`, func() {
			err := ValidateIncludeInvocationRequest(&pb.IncludeInvocationRequest{
				Including: "invocations/a",
				Included:  "invocations/c",
				Overrides: []string{"x"},
			})
			So(err, ShouldErrLike, `overridden_included_invocation: does not match`)
		})

		Convey(`Invalid overriding_included_invocation`, func() {
			err := ValidateIncludeInvocationRequest(&pb.IncludeInvocationRequest{
				Including: "invocations/a",
				Included:  "x",
				Overrides: []string{"invocations/b"},
			})
			So(err, ShouldErrLike, `overriding_included_invocation: does not match`)
		})

		Convey(`include itself`, func() {
			err := ValidateIncludeInvocationRequest(&pb.IncludeInvocationRequest{
				Including: "invocations/a",
				Included:  "invocations/a",
				Overrides: []string{"invocations/b"},
			})
			So(err, ShouldErrLike, `cannot include itself`)
		})

		Convey(`override itself`, func() {
			err := ValidateIncludeInvocationRequest(&pb.IncludeInvocationRequest{
				Including: "invocations/a",
				Included:  "invocations/b",
				Overrides: []string{"invocations/b"},
			})
			So(err, ShouldErrLike, `cannot override itself`)
		})
	})
}
