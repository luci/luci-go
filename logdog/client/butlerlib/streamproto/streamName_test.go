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

package streamproto

import (
	"encoding/json"
	"flag"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestStreamName(t *testing.T) {
	Convey(`A StreamNameFlag`, t, func() {
		sn := StreamNameFlag("")

		Convey(`When attached to a flag.`, func() {
			fs := flag.FlagSet{}
			fs.Var(&sn, "name", "Test stream name.")

			Convey(`Will successfully parse a valid stream name.`, func() {
				err := fs.Parse([]string{"-name", "test/stream/name"})
				So(err, ShouldBeNil)
				So(sn, ShouldEqual, "test/stream/name")
			})

			Convey(`Will fail to parse an invalid stream name.`, func() {
				err := fs.Parse([]string{"-name", "test;stream;name"})
				So(err, ShouldNotBeNil)
			})
		})

		Convey(`With valid value "test/stream/name"`, func() {
			Convey(`Will marshal into a JSON string.`, func() {
				sn = "test/stream/name"
				d, err := json.Marshal(&sn)
				So(err, ShouldBeNil)
				So(string(d), ShouldEqual, `"test/stream/name"`)

				Convey(`And successfully unmarshal.`, func() {
					sn = ""
					err := json.Unmarshal(d, &sn)
					So(err, ShouldBeNil)
					So(sn, ShouldEqual, "test/stream/name")
				})
			})
		})

		Convey(`JSON with an invalid value "test;stream;name" will fail to Unmarshal.`, func() {
			sn := StreamNameFlag("")
			err := json.Unmarshal([]byte(`"test;stream;name"`), &sn)
			So(err, ShouldNotBeNil)
		})
	})
}
