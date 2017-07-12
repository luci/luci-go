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

func TestDuration(t *testing.T) {
	t.Parallel()

	Convey(`A Duration flag`, t, func() {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		var d Duration
		fs.Var(&d, "duration", "Test duration parameter.")

		Convey(`Parses a 10-second Duration from "10s".`, func() {
			err := fs.Parse([]string{"-duration", "10s"})
			So(err, ShouldBeNil)
			So(d, ShouldEqual, time.Second*10)
			So(d.IsZero(), ShouldBeFalse)
		})

		Convey(`Returns an error when parsing "10z".`, func() {
			err := fs.Parse([]string{"-duration", "10z"})
			So(err, ShouldNotBeNil)
		})

		Convey(`When treated as a JSON field`, func() {
			var s struct {
				D Duration `json:"duration"`
			}

			testJSON := `{"duration":10}`
			Convey(`Fails to unmarshal from `+testJSON+`.`, func() {
				testJSON := testJSON
				err := json.Unmarshal([]byte(testJSON), &s)
				So(err, ShouldNotBeNil)
			})

			Convey(`Marshals correctly to duration string.`, func() {
				testDuration := (5 * time.Second) + (2 * time.Minute)
				s.D = Duration(testDuration)
				testJSON, err := json.Marshal(&s)
				So(err, ShouldBeNil)
				So(string(testJSON), ShouldEqual, `{"duration":"2m5s"}`)

				Convey(`And Unmarshals correctly.`, func() {
					s.D = Duration(0)
					err := json.Unmarshal([]byte(testJSON), &s)
					So(err, ShouldBeNil)
					So(s.D, ShouldEqual, testDuration)
				})
			})
		})
	})
}
