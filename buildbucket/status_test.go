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

	"go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
)

func TestStatus(t *testing.T) {
	t.Parallel()

	Convey("ParseStatus", t, func() {
		cases := map[buildbucketpb.Status]*v1.ApiCommonBuildMessage{
			0: {},

			buildbucketpb.Status_SCHEDULED: {
				Status: "SCHEDULED",
			},

			buildbucketpb.Status_STARTED: {
				Status: "STARTED",
			},

			buildbucketpb.Status_SUCCESS: {
				Status: "COMPLETED",
				Result: "SUCCESS",
			},

			buildbucketpb.Status_FAILURE: {
				Status:        "COMPLETED",
				Result:        "FAILURE",
				FailureReason: "BUILD_FAILURE",
			},

			buildbucketpb.Status_INFRA_FAILURE: {
				Status:        "COMPLETED",
				Result:        "FAILURE",
				FailureReason: "INFRA_FAILURE",
			},

			buildbucketpb.Status_CANCELED: {
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
