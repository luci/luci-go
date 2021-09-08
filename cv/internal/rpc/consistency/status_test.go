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

package consistency

import (
	"testing"

	commonpb "go.chromium.org/luci/cv/api/common/v1"
	apiv0pb "go.chromium.org/luci/cv/api/v0"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
)

func TestStatusV1(t *testing.T) {
	t.Parallel()

	Convey("Ensure CV API v1 Run Status matches the internal Status.", t, func() {
		// v1 -> internal.
		for name, val := range commonpb.Run_Status_value {
			name, val := name, val
			Convey("v1."+name, func() {
				other, exists := run.Status_value[name]
				So(exists, ShouldBeTrue)
				So(val, ShouldEqual, other)
			})
		}

		// Internal -> v1.
		for name, val := range run.Status_value {
			name, val := name, val
			Convey("internal."+name, func() {
				other, exists := commonpb.Run_Status_value[name]
				So(exists, ShouldBeTrue)
				So(val, ShouldEqual, other)
			})
		}
	})
}

func TestStatusV0(t *testing.T) {
	t.Parallel()

	Convey("Ensure CV API v0 Run Status matches the internal Status.", t, func() {
		// v0 -> internal.
		for name, val := range apiv0pb.Run_Status_value {
			name, val := name, val
			Convey("v0."+name, func() {
				other, exists := run.Status_value[name]
				So(exists, ShouldBeTrue)
				So(val, ShouldEqual, other)
			})
		}

		// Internal -> v0.
		for name, val := range run.Status_value {
			name, val := name, val
			Convey("internal."+name, func() {
				other, exists := apiv0pb.Run_Status_value[name]
				So(exists, ShouldBeTrue)
				So(val, ShouldEqual, other)
			})
		}
	})
}
