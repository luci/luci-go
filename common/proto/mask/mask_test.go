// Copyright 2020 The LUCI Authors.
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

package mask

import (
	"fmt"
	"testing"

	"go.chromium.org/luci/common/proto/internal/testingpb"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/runtime/protoimpl"

	. "github.com/smartystreets/goconvey/convey"
)

type testMsg = testingpb.Full

var testMsgDescriptor = protoimpl.X.MessageDescriptorOf(&testMsg{})

func TestNormalizePath(t *testing.T) {
	Convey("Retrun empty paths when given empty paths", t, func() {
		So(normalizedPaths([]path{}), ShouldResemble, []path{})
	})
	Convey("Remove all deduplicate paths", t, func() {
		So(normalizedPaths([]path{
			path{"a", "b"},
			path{"a", "b"},
			path{"a", "b"},
		}), ShouldResemble, []path{
			path{"a", "b"},
		})
	})
	Convey("Remove all redundant paths and return sorted", t, func() {
		So(normalizedPaths([]path{
			path{"b", "z"},
			path{"b", "c", "d"},
			path{"b", "c"},
			path{"a"},
		}), ShouldResemble, []path{
			path{"a"},
			path{"b", "c"},
			path{"b", "z"},
		})
	})
}

func TestFromFieldMask(t *testing.T) {
	Convey("From", t, func() {
		parse := func(paths []string, isUpdateMask bool) (mask, error) {
			return FromFieldMask(&field_mask.FieldMask{Paths: paths}, &testMsg{}, false, isUpdateMask)
		}
		// TODO(yiwzhang): ShouldBeResemble will hit infinite loop when comparing
		// descriptor. Comparing the full name of message as a temporary workaround
		var assertMaskEqual func(actual mask, expect mask)
		assertMaskEqual = func(actual mask, expect mask) {
			if expect.descriptor == nil {
				So(actual.descriptor, ShouldBeNil)
			} else {
				So(actual.descriptor, ShouldNotBeNil)
				So(actual.descriptor.FullName(), ShouldEqual, expect.descriptor.FullName())
			}
			So(actual.isRepeated, ShouldEqual, expect.isRepeated)
			So(actual.children, ShouldHaveLength, len(expect.children))
			for seg, expectSubmask := range expect.children {
				So(actual.children, ShouldContainKey, seg)
				assertMaskEqual(actual.children[seg], expectSubmask)
			}
		}

		Convey("empty field mask", func() {
			actual, err := parse([]string{}, false)
			So(err, ShouldBeNil)
			assertMaskEqual(actual, mask{
				descriptor: testMsgDescriptor,
			})
		})

		Convey("field mask with scalar and message fields", func() {
			actual, err := parse([]string{"str", "num", "msg.num"}, false)
			So(err, ShouldBeNil)
			assertMaskEqual(actual, mask{
				descriptor: testMsgDescriptor,
				children: map[string]mask{
					"str": mask{},
					"num": mask{},
					"msg": mask{
						descriptor: testMsgDescriptor,
						children: map[string]mask{
							"num": mask{},
						},
					},
				},
			})
		})
		Convey("field mask with map field", func() {
			actual, err := parse([]string{"map_str_msg.some_key.str", "map_str_num.another_key"}, false)
			So(err, ShouldBeNil)
			assertMaskEqual(actual, mask{
				descriptor: testMsgDescriptor,
				children: map[string]mask{
					"map_str_msg": mask{
						descriptor: testMsgDescriptor.Fields().ByName(protoreflect.Name("map_str_msg")).Message(),
						isRepeated: true,
						children: map[string]mask{
							"some_key": mask{
								descriptor: testMsgDescriptor,
								children: map[string]mask{
									"str": mask{},
								},
							},
						},
					},
					"map_str_num": mask{
						descriptor: testMsgDescriptor.Fields().ByName(protoreflect.Name("map_str_num")).Message(),
						isRepeated: true,
						children: map[string]mask{
							"another_key": mask{},
						},
					},
				},
			})
		})
		Convey("field mask with repeated field", func() {
			actual, err := parse([]string{"nums", "msgs.*.str"}, false)
			So(err, ShouldBeNil)
			assertMaskEqual(actual, mask{
				descriptor: testMsgDescriptor,
				children: map[string]mask{
					"msgs": mask{
						descriptor: testMsgDescriptor,
						isRepeated: true,
						children: map[string]mask{
							"*": mask{
								descriptor: testMsgDescriptor,
								children: map[string]mask{
									"str": mask{},
								},
							},
						},
					},
					"nums": mask{
						isRepeated: true,
					},
				},
			})
		})
		Convey("update mask", func() {
			_, err := parse([]string{"msgs.*.str"}, true)
			So(err, ShouldBeError, fmt.Errorf("update mask allows a repeated field only at the last position; field: msgs is not last"))
			_, err = parse([]string{"map_str_msg.*.str"}, true)
			So(err, ShouldBeError, fmt.Errorf("update mask allows a repeated field only at the last position; field: map_str_msg is not last"))
		})
	})
}
