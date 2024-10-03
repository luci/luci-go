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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	resultpb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestJSON(t *testing.T) {
	ftt.Run(`VariantJSON`, t, func(t *ftt.Test) {
		t.Run(`empty`, func(t *ftt.Test) {
			result, err := VariantJSON(nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Equal("{}"))
		})
		t.Run(`non-empty`, func(t *ftt.Test) {
			variant := &resultpb.Variant{
				Def: map[string]string{
					"builder":           "linux-rel",
					"os":                "Ubuntu-18.04",
					"pathological-case": "\000\001\n\r\f",
				},
			}
			result, err := VariantJSON(variant)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Equal(`{"builder":"linux-rel","os":"Ubuntu-18.04","pathological-case":"\u0000\u0001\n\r\f"}`))
		})
	})
	ftt.Run(`MarshalStructPB`, t, func(t *ftt.Test) {
		t.Run(`empty`, func(t *ftt.Test) {
			result, err := MarshalStructPB(nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Equal("{}"))
		})
		t.Run(`non-empty`, func(t *ftt.Test) {
			values := make(map[string]any)
			values["stringkey"] = "abcdef\000\001\n"
			values["numberkey"] = 123
			values["boolkey"] = true
			values["listkey"] = []any{"a", 9, true}
			pb, err := structpb.NewStruct(values)
			assert.Loosely(t, err, should.BeNil)
			result, err := MarshalStructPB(pb)
			assert.Loosely(t, err, should.BeNil)

			// Different implementations may use different spacing between
			// json elements. Ignore this.
			result = strings.ReplaceAll(result, " ", "")
			assert.Loosely(t, result, should.Equal(`{"boolkey":true,"listkey":["a",9,true],"numberkey":123,"stringkey":"abcdef\u0000\u0001\n"}`))
		})
	})
}
