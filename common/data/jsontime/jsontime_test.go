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

package jsontime

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestJSONTime(t *testing.T) {
	t.Parallel()

	Convey(`Testing JSON time`, t, func() {
		now := Time{time.Date(2017, 1, 2, 3, 4, 5, 67890, time.UTC)}

		for _, z := range []*time.Location{
			time.UTC,
			time.Local,
			time.FixedZone("PST", -8*3600),
		} {
			Convey(fmt.Sprintf(`Can marshal and unmarshal (to UTC) %q.`, z.String()), func() {
				now.Time = now.In(z)

				b, err := json.Marshal(now)
				So(err, ShouldBeNil)

				var t Time
				err = json.Unmarshal(b, &t)
				So(err, ShouldBeNil)
				So(t.Equal(now.Time), ShouldBeTrue)
				So(t.Location(), ShouldEqual, time.UTC)
			})
		}
	})
}
