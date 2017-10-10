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

package buildbot

import (
	"encoding/json"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTime(t *testing.T) {
	t.Parallel()

	Convey("Time can unmarshal JSON it marshalled", t, func() {
		input := &Time{time.Date(2017, 2, 3, 4, 5, 6, 123456789, time.UTC)}
		data, err := json.Marshal(input)
		So(err, ShouldBeNil)
		So(string(data), ShouldEqual, "1486094706.123456")

		var actual Time
		err = json.Unmarshal(data, &actual)
		So(err, ShouldBeNil)
		So(actual.Time, ShouldResemble, input.Time.Truncate(time.Microsecond))
	})

	Convey("Unmarshal close to the next second", t, func() {
		var actual Time
		err := json.Unmarshal([]byte("1486094706.999999"), &actual)
		So(err, ShouldBeNil)
		expected := time.Date(2017, 2, 3, 4, 5, 6, 999999000, time.UTC)
		So(actual.Time, ShouldResemble, expected)
	})
}
