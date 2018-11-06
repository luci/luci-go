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

package milo

import (
	"testing"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/struct"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestExtractProperties(t *testing.T) {
	t.Parallel()

	Convey("ExtractProperties", t, func() {
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
		So(err, ShouldBeNil)

		expected := &structpb.Struct{}
		err = jsonpb.UnmarshalString(`
			{
				"a": 2,
				"b": "str",
				"c": [1]
			}`, expected)
		So(err, ShouldBeNil)

		actual, err := ExtractProperties(root)
		So(err, ShouldBeNil)
		So(actual, ShouldResembleProto, expected)
	})
}
