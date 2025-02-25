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

package msgpackpb

import (
	"reflect"
	"testing"

	"github.com/vmihailenco/msgpack/v5"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestMsgpackPBDeterministicEncod(t *testing.T) {
	t.Parallel()

	ftt.Run(`msgpackpbDeterministicEncode`, t, func(t *ftt.Test) {
		t.Run(`list`, func(t *ftt.Test) {
			val := reflect.ValueOf([]any{
				10,
				"Hello",
				map[string]any{
					"c": "lol",
					"b": []string{"one", "seven"},
					"a": "yo",
				},
				map[int16]string{
					17:  "no",
					71:  "maybe",
					-11: "essentially",
				},
				map[bool]string{
					true:  "ye",
					false: "nah",
				},
				map[uint]string{
					0:  "zero",
					17: "seventeen",
					3:  "three",
				},
				map[int8]string{
					1: "this",
					2: "map",
					3: "is",
					4: "array",
					5: "like",
				},
				-2,
			})
			enc, err := msgpackpbDeterministicEncode(val)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, enc, should.Match(msgpack.RawMessage{
				152,                         // 7 element array
				10,                          //  10
				165, 72, 101, 108, 108, 111, // "Hello"
				131,                    // 3 element map
				161, 97, 162, 121, 111, // "a", "yo"
				161, 98, 146, 163, 111, 110, 101, 165, 115, 101, 118, 101, 110, // "b", ["one", "seven"]
				161, 99, 163, 108, 111, 108, // "c", "lol"
				131,                                                            // 3 element map
				245, 171, 101, 115, 115, 101, 110, 116, 105, 97, 108, 108, 121, // -11, "essentially"
				17, 162, 110, 111, // 17, "no"
				71, 165, 109, 97, 121, 98, 101, // 71, "maybe"
				130,                    // 2 element map
				194, 163, 110, 97, 104, // false, "nah"
				195, 162, 121, 101, // true, "ye"
				131,                        // 3 element map
				0, 164, 122, 101, 114, 111, // 0, "zero"
				3, 165, 116, 104, 114, 101, 101, // 3, "three"
				17, 169, 115, 101, 118, 101, 110, 116, 101, 101, 110, // 17, "seventeen"
				149,                     // 5 element array
				164, 116, 104, 105, 115, // "this"
				163, 109, 97, 112, // "map"
				162, 105, 115, // "is"
				165, 97, 114, 114, 97, 121, // "array"
				164, 108, 105, 107, 101, // "like"
				254, // -2
			}))
		})
	})
}
