// Copyright 2019 The LUCI Authors.
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

package proto

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestFixFieldMasks(t *testing.T) {
	t.Parallel()

	Convey("TestFixFieldMasks", t, func() {
		normalizeJSON := func(jsonData []byte) string {
			buf := &bytes.Buffer{}
			err := json.Indent(buf, jsonData, "", "  ")
			So(err, ShouldBeNil)
			return buf.String()
		}
		testFix := func(messageExample interface{}, jsonMessage, expected string) {
			actual, err := FixFieldMasks([]byte(jsonMessage), reflect.TypeOf(messageExample))
			So(err, ShouldBeNil)
			So(normalizeJSON(actual), ShouldEqual, normalizeJSON([]byte(expected)))
		}
		Convey("No field masks", func() {
			testFix(
				buildbucketpb.GetBuildRequest{},
				`{
					"id": 1
				}`,
				`{
					"id": 1
				}`,
			)
		})
		Convey("Works", func() {
			testFix(
				buildbucketpb.GetBuildRequest{},
				`{
					"fields": "id,createTime"
				}`,
				`{
					"fields": {
						"paths": [
							"id",
							"create_time"
						]
					}
				}`,
			)
		})

		Convey("Nested type", func() {
			testFix(
				buildbucketpb.BatchRequest{},
				`{
					"requests": [
						{
							"getBuild": {
								"fields": "id,createTime"
							}
						}
					]
				}`,
				`{
					"requests": [
						{
							"getBuild": {
								"fields": {
									"paths": [
										"id",
										"create_time"
									]
								}
							}
						}
					]
				}`,
			)
		})

		Convey("invalid field", func() {
			input := `{
				"a": 1
			}`
			_, err := FixFieldMasks([]byte(input), reflect.TypeOf(buildbucketpb.GetBuildRequest{}))
			So(err, ShouldErrLike, `unexpected field path "a"`)
		})

		Convey("invalid field nested", func() {
			input := `{
				"builder": {"a": 1}
			}`
			_, err := FixFieldMasks([]byte(input), reflect.TypeOf(buildbucketpb.GetBuildRequest{}))
			So(err, ShouldErrLike, `unexpected field path "builder.a"`)
		})

		Convey("quotes", func() {
			So(parseFieldMaskString("`a,b`,c"), ShouldResemble, []string{"`a,b`", "c"})
		})

		Convey("two seps", func() {
			So(parseFieldMaskString("a,b,c"), ShouldResemble, []string{"a", "b", "c"})
		})
	})
}
