// Copyright 2018 The LUCI Authors.
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

package buildbucketpb

import (
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes/timestamp"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTimestamps(t *testing.T) {
	t.Parallel()

	Convey("Durations", t, func() {
		build := &Build{
			CreateTime: &timestamp.Timestamp{Seconds: 1000},
			StartTime:  &timestamp.Timestamp{Seconds: 1010},
			EndTime:    &timestamp.Timestamp{Seconds: 1030},
		}
		dur, ok := build.SchedulingDuration()
		So(ok, ShouldBeTrue)
		So(dur, ShouldEqual, 10*time.Second)

		dur, ok = build.RunDuration()
		So(ok, ShouldBeTrue)
		So(dur, ShouldEqual, 20*time.Second)
	})
}
