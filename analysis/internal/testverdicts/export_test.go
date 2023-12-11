// Copyright 2023 The LUCI Authors.
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

package testverdicts

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/types/known/structpb"

	resultpb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func TestVariantJSON(t *testing.T) {
	Convey(`variantJSON`, t, func() {
		Convey(`empty`, func() {
			result, err := variantJSON(nil)
			So(err, ShouldBeNil)
			So(result, ShouldEqual, "{}")
		})
		Convey(`non-empty`, func() {
			variant := &resultpb.Variant{
				Def: map[string]string{
					"builder":           "linux-rel",
					"os":                "Ubuntu-18.04",
					"pathological-case": "\000\001\n\r\f",
				},
			}
			result, err := variantJSON(variant)
			So(err, ShouldBeNil)
			So(result, ShouldEqual, `{"builder":"linux-rel","os":"Ubuntu-18.04","pathological-case":"\u0000\u0001\n\r\u000c"}`)
		})
	})
	Convey(`MarshalStructPB`, t, func() {
		Convey(`empty`, func() {
			result, err := MarshalStructPB(nil)
			So(err, ShouldBeNil)
			So(result, ShouldEqual, "{}")
		})
		Convey(`non-empty`, func() {
			values := make(map[string]interface{})
			values["stringkey"] = "abcdef\000\001\n"
			values["numberkey"] = 123
			values["boolkey"] = true
			values["listkey"] = []interface{}{"a", 9, true}
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
}
