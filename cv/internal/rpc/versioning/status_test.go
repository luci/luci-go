// Copyright 2021 The LUCI Authors.
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

package versioning

import (
	"testing"

	apiv0pb "go.chromium.org/luci/cv/api/v0"
	apiv1pb "go.chromium.org/luci/cv/api/v1"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"

	. "github.com/smartystreets/goconvey/convey"
)

func TestStatusV1(t *testing.T) {
	t.Parallel()

	Convey("RunStatusV1 returns a valid enum", t, func() {
		for name, val := range run.Status_value {
			name, val := name, val
			Convey("for internal."+name, func() {
				eq := RunStatusV1(run.Status(val))

				// check if it's typed with the API enum.
				So(eq.Descriptor().FullName(), ShouldEqual,
					apiv1pb.Run_STATUS_UNSPECIFIED.Descriptor().FullName())
				// check if it's one of the defined enum ints.
				_, ok := apiv1pb.Run_Status_name[int32(eq)]
				So(ok, ShouldBeTrue)
			})
		}
	})
}

func TestStatusV0(t *testing.T) {
	t.Parallel()

	Convey("RunStatusV0 returns a valid enum", t, func() {
		for name, val := range run.Status_value {
			name, val := name, val
			Convey("for internal."+name, func() {
				eq := RunStatusV0(run.Status(val))

				// check if it's typed with the API enum.
				So(eq.Descriptor().FullName(), ShouldEqual,
					apiv0pb.Run_STATUS_UNSPECIFIED.Descriptor().FullName())
				// check if it's one of the defined enum ints.
				_, ok := apiv0pb.Run_Status_name[int32(eq)]
				So(ok, ShouldBeTrue)
			})
		}
	})

	Convey("TryjobStatusV0 returns a valid enum", t, func() {
		for name, val := range tryjob.Status_value {
			name, val := name, val
			Convey("for internal."+name, func() {
				eq := LegacyTryjobStatusV0(tryjob.Status(val))

				// check if it's typed with the API enum.
				So(eq.Descriptor().FullName(), ShouldEqual,
					apiv0pb.Tryjob_STATUS_UNSPECIFIED.Descriptor().FullName())
				// check if it's one of the defined enum ints.
				_, ok := apiv0pb.Tryjob_Status_name[int32(eq)]
				So(ok, ShouldBeTrue)
			})
		}
	})

	Convey("TryjobResultStatusV0 returns a valid enum", t, func() {
		for name, val := range tryjob.Result_Status_value {
			name, val := name, val
			Convey("for internal."+name, func() {
				eq := LegacyTryjobResultStatusV0(tryjob.Result_Status(val))

				// check if it's typed with the API enum.
				So(eq.Descriptor().FullName(), ShouldEqual,
					apiv0pb.Tryjob_Result_RESULT_STATUS_UNSPECIFIED.Descriptor().FullName())
				// check if it's one of the defined enum ints.
				_, ok := apiv0pb.Tryjob_Result_Status_name[int32(eq)]
				So(ok, ShouldBeTrue)
			})
		}
	})
}
