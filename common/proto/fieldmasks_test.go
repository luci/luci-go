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

	jsonpbv1 "github.com/golang/protobuf/jsonpb"
	protov1 "github.com/golang/protobuf/proto"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/proto/internal/testingpb"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestFixFieldMasks(t *testing.T) {
	t.Parallel()

	ftt.Run("TestFixFieldMasks", t, func(t *ftt.Test) {
		normalizeJSON := func(jsonData []byte) string {
			buf := &bytes.Buffer{}
			err := json.Indent(buf, jsonData, "", "  ")
			assert.Loosely(t, err, should.BeNil)
			return buf.String()
		}
		testFix := func(pb proto.Message, expected string) {
			typ := reflect.TypeOf(pb).Elem()

			actual, err := protojson.Marshal(pb)
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, normalizeJSON(actual), should.Equal(normalizeJSON([]byte(expected))))

			jsBadEmulated, err := fixFieldMasksBeforeUnmarshal([]byte(actual), typ)
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, jsonpbv1.UnmarshalString(string(jsBadEmulated), protov1.MessageV1(pb)), should.BeNil)
		}
		t.Run("No field masks", func(t *ftt.Test) {
			testFix(
				&testingpb.Simple{Id: 1},
				`{
					"id": "1"
				}`,
			)
		})

		t.Run("Works", func(t *ftt.Test) {
			testFix(
				&testingpb.Simple{Fields: &field_mask.FieldMask{Paths: []string{
					"id", "some_field",
				}}},
				`{
					"fields": "id,someField"
				}`,
			)
		})

		t.Run("Properties", func(t *ftt.Test) {
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

		t.Run("Nested type", func(t *ftt.Test) {
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

		t.Run("invalid field", func(t *ftt.Test) {
			input := `{
				"a": 1
			}`
			_, err := fixFieldMasksBeforeUnmarshal([]byte(input), reflect.TypeOf(testingpb.Simple{}))
			assert.Loosely(t, err, should.ErrLike(`unexpected field path "a"`))
		})

		t.Run("invalid field nested", func(t *ftt.Test) {
			input := `{
				"some": {"a": 1}
			}`
			_, err := fixFieldMasksBeforeUnmarshal([]byte(input), reflect.TypeOf(testingpb.Simple{}))
			assert.Loosely(t, err, should.ErrLike(`unexpected field path "some.a"`))
		})

		t.Run("quotes", func(t *ftt.Test) {
			assert.Loosely(t, parseFieldMaskString("`a,b`,c"), should.Resemble([]string{"`a,b`", "c"}))
		})

		t.Run("two seps", func(t *ftt.Test) {
			assert.Loosely(t, parseFieldMaskString("a,b,c"), should.Resemble([]string{"a", "b", "c"}))
		})
	})
}
