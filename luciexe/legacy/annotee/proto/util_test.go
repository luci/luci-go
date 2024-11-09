// Copyright 2018 The LUCI Authors.
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

package annopb

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestExtractProperties(t *testing.T) {
	t.Parallel()

	ftt.Run("ExtractProperties", t, func(t *ftt.Test) {
		root := &Step{}
		err := proto.UnmarshalText(`
			substep {
				step {
					property {
						name: "a"
						value: "1"
					}
					property {
						name: "b"
						value: "\"str\""
					}
				}
			}
			substep {
				step {
					property {
						name: "a"
						value: "2"
					}
					property {
						name: "c"
						value: "[1]"
					}
				}
			}`, root)
		assert.Loosely(t, err, should.BeNil)

		expected := &structpb.Struct{}
		err = protojson.Unmarshal([]byte(`
			{
				"a": 2,
				"b": "str",
				"c": [1]
			}`), expected)
		assert.Loosely(t, err, should.BeNil)

		actual, err := ExtractProperties(root)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actual, should.Resemble(expected))
	})
}
