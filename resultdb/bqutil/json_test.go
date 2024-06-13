// Copyright 2024 The LUCI Authors.
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

package bqutil

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/types/known/structpb"

	. "github.com/smartystreets/goconvey/convey"
)

func TestJSON(t *testing.T) {
	Convey(`MarshalStructPB`, t, func() {
		Convey(`empty`, func() {
			result, err := MarshalStructPB(nil)
			So(err, ShouldBeNil)
			So(result, ShouldEqual, "{}")
		})
		Convey(`non-empty`, func() {
			values := map[string]interface{}{
				"stringkey": "abcdef\000\001\n",
				"numberkey": 123,
				"boolkey":   true,
				"listkey":   []interface{}{"a", 9, true},
			}
			pb, err := structpb.NewStruct(values)
			So(err, ShouldBeNil)
			result, err := MarshalStructPB(pb)
			So(err, ShouldBeNil)

			// Different implementations may use different spacing between
			// json elements. Ignore this.
			result = strings.ReplaceAll(result, " ", "")
			So(result, ShouldEqual, `{"boolkey":true,"listkey":["a",9,true],"numberkey":123,"stringkey":"abcdef\u0000\u0001\n"}`)
		})
	})

	Convey(`MarshalStringStructPBMap`, t, func() {
		Convey(`empty`, func() {
			result, err := MarshalStringStructPBMap(nil)
			So(err, ShouldBeNil)
			So(result, ShouldEqual, "{}")
		})
		Convey(`non-empty`, func() {
			values := map[string]interface{}{
				"stringkey": "abcdef\000\001\n",
				"numberkey": 123,
				"boolkey":   true,
				"listkey":   []interface{}{"a", 9, true},
			}
			pb, err := structpb.NewStruct(values)
			So(err, ShouldBeNil)
			m := map[string]*structpb.Struct{
				"a_key": pb,
			}
			result, err := MarshalStringStructPBMap(m)
			So(err, ShouldBeNil)

			// Different implementations may use different spacing between
			// json elements. Ignore this.
			result = strings.ReplaceAll(result, " ", "")
			So(result, ShouldEqual, `{"a_key":{"boolkey":true,"listkey":["a",9,true],"numberkey":123,"stringkey":"abcdef\u0000\u0001\n"}}`)
		})
	})
}
