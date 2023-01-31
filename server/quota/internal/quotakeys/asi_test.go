// Copyright 2022 The LUCI Authors.
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

package quotakeys

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestASI(t *testing.T) {
	t.Parallel()

	s := func(strs ...string) []string {
		return strs
	}

	Convey(`ASI`, t, func() {
		Convey(`happy path`, func() {
			tests := []struct {
				name string
				in   []string
				out  string
			}{
				{name: "empty"},
				{name: "single (basic)", in: s("hello"), out: "hello"},
				{name: "multi (basic)", in: s("hello", "there"), out: "hello|there"},
				{name: "single (enc)", in: s("~single~"), out: "{IWK4@B5D.."},
				{name: "multi (enc)", in: s("stuff", "{escape pls}", "more"), out: "stuff|{HY%8.@;od#E,9TD|more"},
			}

			for _, tc := range tests {
				tc := tc
				Convey(tc.name, func() {
					SoMsg("encode", AssembleASI(tc.in...), ShouldResemble, tc.out)
					dec, err := DecodeASI(tc.out)
					So(err, ShouldBeNil)
					SoMsg("decode", dec, ShouldResemble, tc.in)
				})
			}
		})
		Convey(`error`, func() {
			_, err := DecodeASI("{notre~lascii85")
			So(err, ShouldErrLike, "DecodeASI: section[0]: illegal ascii85 data at input byte 5")
		})
	})
}
