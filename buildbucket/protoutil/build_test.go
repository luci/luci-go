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

package protoutil

import (
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes/timestamp"

	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTimestamps(t *testing.T) {
	t.Parallel()

	Convey("Durations", t, func() {
		build := &pb.Build{
			CreateTime: &timestamp.Timestamp{Seconds: 1000},
			StartTime:  &timestamp.Timestamp{Seconds: 1010},
			EndTime:    &timestamp.Timestamp{Seconds: 1030},
		}
		dur, ok := SchedulingDuration(build)
		So(ok, ShouldBeTrue)
		So(dur, ShouldEqual, 10*time.Second)

		dur, ok = RunDuration(build)
		So(ok, ShouldBeTrue)
		So(dur, ShouldEqual, 20*time.Second)
	})
}
