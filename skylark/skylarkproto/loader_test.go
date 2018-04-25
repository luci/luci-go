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

package skylarkproto

import (
	"sort"
	"testing"

	"github.com/google/skylark"
	"github.com/google/skylark/skylarkstruct"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLoader(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		dict, err := LoadProtoModule("go.chromium.org/luci/skylark/skylarkproto/testprotos/test.proto")
		So(err, ShouldBeNil)
		So(len(dict), ShouldEqual, 1)

		So(dict["testprotos"], ShouldHaveSameTypeAs, &skylarkstruct.Struct{})
		symbols := dict["testprotos"].(*skylarkstruct.Struct)

		// 'symbols' struct contains all top-level symbols discovered in the proto
		// file. The test will break and will need to be updated whenever something
		// new is added to test.proto. Note that symbols from another.proto do not
		// appear here (by design).
		dict = skylark.StringDict{}
		symbols.ToStringDict(dict)
		keys := []string{}
		for k := range dict {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		So(keys, ShouldResemble, []string{
			"Complex",
			"IntFields",
			"MessageFields",
			"Simple",
		})
	})

	Convey("Unknown protos", t, func() {
		_, err := LoadProtoModule("unknown.proto")
		So(err.Error(), ShouldEqual, "no such proto file registered")
	})
}
