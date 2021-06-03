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

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"

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
		testFix := func(pb proto.Message, expected string) {
			typ := reflect.TypeOf(pb).Elem()

			actual, err := protojson.Marshal(proto.MessageV2(pb))
			So(err, ShouldBeNil)

			So(normalizeJSON(actual), ShouldEqual, normalizeJSON([]byte(expected)))

			jsBadEmulated, err := FixFieldMasksBeforeUnmarshal([]byte(actual), typ)
			So(err, ShouldBeNil)

			So(jsonpb.UnmarshalString(string(jsBadEmulated), pb), ShouldBeNil)
		}
		Convey("No field masks", func() {
			testFix(
				&testingpb.Simple{Id: 1},
				`{
					"id": "1"
				}`,
			)
		})

		Convey("Works", func() {
			testFix(
				&testingpb.Simple{Fields: &field_mask.FieldMask{Paths: []string{
					"id", "some_field",
				}}},
				`{
					"fields": "id,someField"
				}`,
			)
		})

		Convey("Properties", func() {
			testFix(
				&testingpb.Props{
					Properties: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"foo": {Kind: &structpb.Value_StringValue{StringValue: "bar"}},
						},
					},
				},
				`{
						"properties": {
							"foo": "bar"
						}
					}`,
			)
		})

		Convey("Nested type", func() {
			testFix(
				&testingpb.WithInner{
					Msgs: []*testingpb.WithInner_Inner{
						{
							Msg: &testingpb.WithInner_Inner_Simple{
								Simple: &testingpb.Simple{Fields: &field_mask.FieldMask{Paths: []string{
									"id", "some_field",
								}}},
							},
						},
					},
				},
				`{
						"msgs": [
							{
								"simple": {
									"fields": "id,someField"
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
			_, err := FixFieldMasksBeforeUnmarshal([]byte(input), reflect.TypeOf(testingpb.Simple{}))
			So(err, ShouldErrLike, `unexpected field path "a"`)
		})

		Convey("invalid field nested", func() {
			input := `{
				"some": {"a": 1}
			}`
			_, err := FixFieldMasksBeforeUnmarshal([]byte(input), reflect.TypeOf(testingpb.Simple{}))
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
