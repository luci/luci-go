// Copyright 2015 The LUCI Authors.
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

package clockflag

import (
	"encoding/json"
	"flag"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTime(t *testing.T) {
	t.Parallel()

	Convey(`A Time flag`, t, func() {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		var d Time
		fs.Var(&d, "time", "Test time parameter.")

		Convey(`Parses a 10-second Time from "2015-05-05T23:47:17+00:00".`, func() {
			err := fs.Parse([]string{"-time", "2015-05-05T23:47:17+00:00"})
			So(err, ShouldBeNil)
			So(d.Time().Equal(time.Unix(1430869637, 0)), ShouldBeTrue)
		})

		Convey(`Returns an error when parsing "asdf".`, func() {
			err := fs.Parse([]string{"-time", "asdf"})
			So(err, ShouldNotBeNil)
		})

		Convey(`When treated as a JSON field`, func() {
			var s struct {
				T Time `json:"time"`
			}

			testJSON := `{"time":"asdf"}`
			Convey(`Fails to unmarshal from `+testJSON+`.`, func() {
				testJSON := testJSON
				err := json.Unmarshal([]byte(testJSON), &s)
				So(err, ShouldNotBeNil)
			})

			Convey(`Marshals correctly to RFC3339 time string.`, func() {
				s.T = Time(time.Unix(1430869637, 0))
				testJSON, err := json.Marshal(&s)
				So(err, ShouldBeNil)
				So(string(testJSON), ShouldEqual, `{"time":"2015-05-05T23:47:17Z"}`)

				Convey(`And Unmarshals correctly.`, func() {
					s.T = Time{}
					err := json.Unmarshal([]byte(testJSON), &s)
					So(err, ShouldBeNil)
					So(s.T.Time().Equal(time.Unix(1430869637, 0)), ShouldBeTrue)
				})
			})
		})
	})
}
