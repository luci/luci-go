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

package common

import (
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"google.golang.org/protobuf/types/known/timestamppb"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestPB2Time(t *testing.T) {
	t.Parallel()

	Convey("RoundTrip", t, func() {
		Convey("Specified", func() {
			ts := testclock.TestRecentTimeUTC
			pb := Time2PBNillable(ts)
			So(pb, ShouldResembleProto, Time2PBNillable(ts))
			So(pb, ShouldResembleProto, timestamppb.New(ts))
			So(PB2TimeNillable(pb), ShouldEqual, ts)
		})
		Convey("Zero / nil", func() {
			So(PB2TimeNillable(nil), ShouldEqual, time.Time{})
			So(Time2PBNillable(time.Time{}), ShouldBeNil)
		})
	})
}
