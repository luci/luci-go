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

package buildbucket

import (
	"testing"

	v1 "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func TestStatus(t *testing.T) {
	t.Parallel()

	Convey("ParseStatus", t, func() {
		cases := map[Status]*v1.ApiCommonBuildMessage{
			0: {},

			StatusScheduled: {
				Status: "SCHEDULED",
			},

			StatusStarted: {
				Status: "STARTED",
			},

			StatusSuccess: {
				Status: "COMPLETED",
				Result: "SUCCESS",
			},

			StatusFailure: {
				Status:        "COMPLETED",
				Result:        "FAILURE",
				FailureReason: "BUILD_FAILURE",
			},

			StatusError: {
				Status:        "COMPLETED",
				Result:        "FAILURE",
				FailureReason: "INFRA_FAILURE",
			},

			StatusCancelled: {
				Status:            "COMPLETED",
				Result:            "CANCELED",
				CancelationReason: "CANCELED_EXPLICITLY",
			},
		}
		for expected, build := range cases {
			expected := expected
			build := build
			Convey(expected.String(), func() {
				actual, err := ParseStatus(build)
				So(err, ShouldBeNil)
				So(actual, ShouldEqual, expected)
			})
		}
	})
}
