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

package starlarkproto

import (
	"sort"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLoader(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		dict, err := LoadProtoModule("go.chromium.org/luci/starlark/starlarkproto/testprotos/test.proto")
		So(err, ShouldBeNil)
		So(len(dict), ShouldEqual, 1)

		So(dict["testprotos"], ShouldHaveSameTypeAs, &starlarkstruct.Struct{})
		symbols := dict["testprotos"].(*starlarkstruct.Struct)

		// 'symbols' struct contains all top-level symbols discovered in the proto
		// file. The test will break and will need to be updated whenever something
		// new is added to test.proto. Note that symbols from another.proto do not
		// appear here (by design).
		dict = starlark.StringDict{}
		symbols.ToStringDict(dict)
		keys := []string{}
		for k := range dict {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		So(keys, ShouldResemble, []string{
			"AnotherSimple",
			"Complex",
			"ENUM_DEFAULT",
			"ENUM_VAL_1",
			"MapWithMessageType",
			"MapWithPrimitiveType",
			"MessageFields",
			"RefsOtherProtos",
			"Simple",
			"SimpleFields",
		})
	})

	Convey("Unknown protos", t, func() {
		_, err := LoadProtoModule("unknown.proto")
		So(err.Error(), ShouldEqual, "no such proto file registered")
	})
}
