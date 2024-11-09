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

package reflectutil

import (
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestPath(t *testing.T) {
	t.Parallel()

	ftt.Run(`Path`, t, func(t *ftt.Test) {
		t.Run(`can be constructed from fields`, func(t *ftt.Test) {
			desc := (&TestPathMessage{}).ProtoReflect().Descriptor()
			fieldA := desc.Fields().ByName("single_inner")
			fieldB := fieldA.Message().Fields().ByName("str")

			pth := Path{
				PathField{fieldA},
				PathField{fieldB},
			}

			assert.Loosely(t, pth.String(), should.Match(".single_inner.str"))

			msg := &TestPathMessage{SingleInner: &TestPathMessage_Inner{Str: "sup"}}
			assert.Loosely(t, pth.Retrieve(msg), should.Resemble(protoreflect.ValueOf("sup")))
		})

		t.Run(`supports string map fields`, func(t *ftt.Test) {
			desc := (&TestPathMessage{}).ProtoReflect().Descriptor()
			fieldA := desc.Fields().ByName("map_inner")
			fieldB := fieldA.MapValue().Message().Fields().ByName("str")

			pth := Path{
				PathField{fieldA},
				PathMapKey(protoreflect.MapKey(protoreflect.ValueOf("neat"))),
				PathField{fieldB},
			}

			assert.Loosely(t, pth.String(), should.Match(".map_inner[\"neat\"].str"))

			msg := &TestPathMessage{MapInner: map[string]*TestPathMessage_Inner{
				"neat": {Str: "sup"}},
			}
			assert.Loosely(t, pth.Retrieve(msg), should.Resemble(protoreflect.ValueOf("sup")))
		})

		t.Run(`supports integer map fields`, func(t *ftt.Test) {
			desc := (&TestPathMessage{}).ProtoReflect().Descriptor()
			fieldA := desc.Fields().ByName("int_map_inner")
			fieldB := fieldA.MapValue().Message().Fields().ByName("str")

			pth := Path{
				PathField{fieldA},
				PathMapKey(protoreflect.ValueOfInt32(100).MapKey()),
				PathField{fieldB},
			}

			assert.Loosely(t, pth.String(), should.Match(".int_map_inner[100].str"))

			msg := &TestPathMessage{IntMapInner: map[int32]*TestPathMessage_Inner{
				100: {Str: "sup"}},
			}
			assert.Loosely(t, pth.Retrieve(msg), should.Resemble(protoreflect.ValueOf("sup")))
		})

		t.Run(`supports repeated fields`, func(t *ftt.Test) {
			desc := (&TestPathMessage{}).ProtoReflect().Descriptor()
			fieldA := desc.Fields().ByName("multi_inner")
			fieldB := fieldA.Message().Fields().ByName("str")

			pth := Path{
				PathField{fieldA},
				PathListIdx(2),
				PathField{fieldB},
			}

			assert.Loosely(t, pth.String(), should.Match(".multi_inner[2].str"))

			msg := &TestPathMessage{MultiInner: []*TestPathMessage_Inner{
				{Str: "nope"},
				{Str: "nope"},
				{Str: "sup"},
			}}
			assert.Loosely(t, pth.Retrieve(msg), should.Resemble(protoreflect.ValueOf("sup")))
		})
	})
}
