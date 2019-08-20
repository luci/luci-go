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

	"go.chromium.org/luci/common/proto/internal/testingpb"

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
				testingpb.Simple{},
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
				testingpb.Simple{},
				`{
					"fields": "id,someField"
				}`,
				`{
					"fields": {
						"paths": [
							"id",
							"some_field"
						]
					}
				}`,
			)
		})

		Convey("Properties", func() {
			testFix(
				testingpb.Props{},
				`{
					"properties": {
						"foo": "bar"
					}
				}`,
				`{
					"properties": {
						"foo": "bar"
					}
				}`,
			)
		})

		Convey("Nested type", func() {
			testFix(
				testingpb.WithInner{},
				`{
					"msgs": [
						{
							"simple": {
								"fields": "id,someField"
							}
						}
					]
				}`,
				`{
					"msgs": [
						{
							"simple": {
								"fields": {
									"paths": [
										"id",
										"some_field"
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
			_, err := FixFieldMasks([]byte(input), reflect.TypeOf(testingpb.Simple{}))
			So(err, ShouldErrLike, `unexpected field path "a"`)
		})

		Convey("invalid field nested", func() {
			input := `{
				"some": {"a": 1}
			}`
			_, err := FixFieldMasks([]byte(input), reflect.TypeOf(testingpb.Simple{}))
			So(err, ShouldErrLike, `unexpected field path "some.a"`)
		})

		Convey("quotes", func() {
			So(parseFieldMaskString("`a,b`,c"), ShouldResemble, []string{"`a,b`", "c"})
		})

		Convey("two seps", func() {
			So(parseFieldMaskString("a,b,c"), ShouldResemble, []string{"a", "b", "c"})
		})
	})
}
