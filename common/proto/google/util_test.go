// Copyright 2016 The LUCI Authors.
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

package google

import (
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTimestamp(t *testing.T) {
	t.Parallel()

	Convey(`Can convert to/from time.Time instances.`, t, func() {
		for _, v := range []time.Time{
			{},
			testclock.TestTimeLocal,
			testclock.TestTimeUTC,
		} {
			So(TimeFromProto(NewTimestamp(v)).UTC(), ShouldResemble, v.UTC())
		}
	})

	Convey(`A zero time.Time produces a nil Timestamp.`, t, func() {
		So(NewTimestamp(time.Time{}), ShouldBeNil)
	})

	Convey(`A nil Timestamp produces a zero time.Time.`, t, func() {
		So(TimeFromProto(nil).IsZero(), ShouldBeTrue)
	})
}

func TestDuration(t *testing.T) {
	t.Parallel()

	Convey(`Can convert to/from time.Duration instances.`, t, func() {
		for _, v := range []time.Duration{
			-10 * time.Second,
			0,
			10 * time.Second,
		} {
			So(DurationFromProto(NewDuration(v)), ShouldEqual, v)
		}
	})

	Convey(`A zero time.Duration produces a nil Duration.`, t, func() {
		So(NewDuration(0), ShouldBeNil)
	})

	Convey(`A nil Duration produces a zero time.Duration.`, t, func() {
		So(DurationFromProto(nil), ShouldEqual, time.Duration(0))
	})
}
