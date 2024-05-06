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
	"testing"

	"github.com/golang/protobuf/proto"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.chromium.org/luci/common/proto/internal/testingpb"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

type testMsg = testingpb.Full

var testMsgDescriptor = proto.MessageReflect(&testMsg{}).Descriptor()

func TestNormalizePath(t *testing.T) {
	Convey("Retrun empty paths when given empty paths", t, func() {
		So(normalizePaths([]path{}), ShouldResemble, []path{})
	})
	Convey("Remove all deduplicate paths", t, func() {
		So(normalizePaths([]path{
			{"a", "b"},
			{"a", "b"},
			{"a", "b"},
		}), ShouldResemble, []path{
			{"a", "b"},
		})
	})
	Convey("Remove all redundant paths and return sorted", t, func() {
		So(normalizePaths([]path{
			{"b", "z"},
			{"b", "c", "d"},
			{"b", "c"},
			{"a"},
		}), ShouldResemble, []path{
			{"a"},
			{"b", "c"},
			{"b", "z"},
		})
	})
}

func TestFromFieldMask(t *testing.T) {
	Convey("From", t, func() {
		parse := func(paths []string, isUpdateMask bool) (*Mask, error) {
			return FromFieldMask(&field_mask.FieldMask{Paths: paths}, &testMsg{}, false, isUpdateMask)
		}

		Convey("empty field mask", func() {
			actual, err := parse([]string{}, false)
			So(err, ShouldBeNil)
			assertMaskEqual(actual, &Mask{
				descriptor: testMsgDescriptor,
			})
		})

		Convey("field mask with scalar and message fields", func() {
			actual, err := parse([]string{"str", "num", "msg.num"}, false)
			So(err, ShouldBeNil)
			assertMaskEqual(actual, &Mask{
				descriptor: testMsgDescriptor,
				children: map[string]*Mask{
					"str": {},
					"num": {},
					"msg": {
						descriptor: testMsgDescriptor,
						children: map[string]*Mask{
							"num": {},
						},
					},
				},
			})
		})
		Convey("field mask with map field", func() {
			actual, err := parse([]string{"map_str_msg.some_key.str", "map_str_num.another_key"}, false)
			So(err, ShouldBeNil)
			assertMaskEqual(actual, &Mask{
				descriptor: testMsgDescriptor,
				children: map[string]*Mask{
					"map_str_msg": {
						descriptor: testMsgDescriptor.Fields().ByName(protoreflect.Name("map_str_msg")).Message(),
						isRepeated: true,
						children: map[string]*Mask{
							"some_key": {
								descriptor: testMsgDescriptor,
								children: map[string]*Mask{
									"str": {},
								},
							},
						},
					},
					"map_str_num": {
						descriptor: testMsgDescriptor.Fields().ByName(protoreflect.Name("map_str_num")).Message(),
						isRepeated: true,
						children: map[string]*Mask{
							"another_key": {},
						},
					},
				},
			})
		})
		Convey("field mask with repeated field", func() {
			actual, err := parse([]string{"nums", "msgs.*.str"}, false)
			So(err, ShouldBeNil)
			assertMaskEqual(actual, &Mask{
				descriptor: testMsgDescriptor,
				children: map[string]*Mask{
					"msgs": {
						descriptor: testMsgDescriptor,
						isRepeated: true,
						children: map[string]*Mask{
							"*": {
								descriptor: testMsgDescriptor,
								children: map[string]*Mask{
									"str": {},
								},
							},
						},
					},
					"nums": {
						isRepeated: true,
					},
				},
			})
		})
		Convey("update mask", func() {
			_, err := parse([]string{"msgs.*.str"}, true)
			So(err, ShouldErrLike, "update mask allows a repeated field only at the last position; field: msgs is not last")
			_, err = parse([]string{"map_str_msg.*.str"}, true)
			So(err, ShouldErrLike, "update mask allows a repeated field only at the last position; field: map_str_msg is not last")
			_, err = parse([]string{"map_str_num.some_key"}, true)
			So(err, ShouldBeNil)
		})
	})
}
func TestTrim(t *testing.T) {
	Convey("Test", t, func() {
		testTrim := func(maskPaths []string, msg proto.Message) {
			m, err := FromFieldMask(&field_mask.FieldMask{Paths: maskPaths}, &testMsg{}, false, false)
			So(err, ShouldBeNil)
			err = m.Trim(msg)
			So(err, ShouldBeNil)
		}
		Convey("trim scalar field", func() {
			msg := &testMsg{Num: 1}
			testTrim([]string{"str"}, msg)
			So(msg, ShouldResembleProto, &testMsg{})
		})
		Convey("keep scalar field", func() {
			msg := &testMsg{Num: 1}
			testTrim([]string{"num"}, msg)
			So(msg, ShouldResembleProto, &testMsg{Num: 1})
		})
		Convey("trim repeated scalar field", func() {
			msg := &testMsg{Nums: []int32{1, 2}}
			testTrim([]string{"str"}, msg)
			So(msg, ShouldResembleProto, &testMsg{})
		})
		Convey("keep repeated scalar field", func() {
			msg := &testMsg{Nums: []int32{1, 2}}
			testTrim([]string{"nums"}, msg)
			So(msg, ShouldResembleProto, &testMsg{Nums: []int32{1, 2}})
		})
		Convey("trim submessage", func() {
			msg := &testMsg{
				Msg: &testMsg{
					Num: 1,
				},
			}
			testTrim([]string{"str"}, msg)
			So(msg, ShouldResembleProto, &testMsg{})
		})
		Convey("keep submessage entirely", func() {
			msg := &testMsg{
				Msg: &testMsg{
					Num: 1,
					Str: "abc",
				},
			}
			testTrim([]string{"msg"}, msg)
			So(msg, ShouldResembleProto, &testMsg{
				Msg: &testMsg{
					Num: 1,
					Str: "abc",
				},
			})
		})
		Convey("keep submessage partially", func() {
			msg := &testMsg{
				Msg: &testMsg{
					Num: 1,
					Str: "abc",
				},
			}
			testTrim([]string{"msg.str"}, msg)
			So(msg, ShouldResembleProto, &testMsg{
				Msg: &testMsg{
					Str: "abc",
				},
			})
		})
		Convey("trim repeated message field", func() {
			msg := &testMsg{
				Msgs: []*testMsg{
					{Num: 1},
					{Num: 2},
				},
			}
			testTrim([]string{"str"}, msg)
			So(msg, ShouldResembleProto, &testMsg{})
		})
		Convey("keep subfield of repeated message field entirely", func() {
			msg := &testMsg{
				Msgs: []*testMsg{
					{Num: 1, Str: "abc"},
					{Num: 2, Str: "bcd"},
				},
			}
			testTrim([]string{"msgs"}, msg)
			So(msg, ShouldResembleProto, &testMsg{
				Msgs: []*testMsg{
					{Num: 1, Str: "abc"},
					{Num: 2, Str: "bcd"},
				},
			})
		})
		Convey("keep subfield of repeated message field partially", func() {
			msg := &testMsg{
				Msgs: []*testMsg{
					{Num: 1, Str: "abc"},
					{Num: 2, Str: "bcd"},
				},
			}
			testTrim([]string{"msgs.*.str"}, msg)
			So(msg, ShouldResembleProto, &testMsg{
				Msgs: []*testMsg{
					{Str: "abc"},
					{Str: "bcd"},
				},
			})
		})
		Convey("trim map field", func() {
			msg := &testMsg{
				MapStrNum: map[string]int32{"a": 1},
			}
			testTrim([]string{"str"}, msg)
			So(msg, ShouldResembleProto, &testMsg{})
		})
		Convey("trim map (scalar value)", func() {
			msg := &testMsg{
				MapStrNum: map[string]int32{
					"a": 1,
					"b": 2,
				},
			}
			testTrim([]string{"map_str_num.a"}, msg)
			So(msg, ShouldResembleProto, &testMsg{
				MapStrNum: map[string]int32{
					"a": 1,
				},
			})
		})
		Convey("trim map (message value) ", func() {
			msg := &testMsg{
				MapStrMsg: map[string]*testMsg{
					"a": {Num: 1},
					"b": {Num: 2},
				},
			}
			testTrim([]string{"map_str_msg.a"}, msg)
			So(msg, ShouldResembleProto, &testMsg{
				MapStrMsg: map[string]*testMsg{
					"a": {Num: 1},
				},
			})
		})
		Convey("trim map (message value) and keep message value partially", func() {
			msg := &testMsg{
				MapStrMsg: map[string]*testMsg{
					"a": {Num: 1, Str: "abc"},
					"b": {Num: 2, Str: "bcd"},
				},
			}
			testTrim([]string{"map_str_msg.a.num"}, msg)
			So(msg, ShouldResembleProto, &testMsg{
				MapStrMsg: map[string]*testMsg{
					"a": {Num: 1},
				},
			})
		})
		Convey("trim map (message value) with star key and keep message value partially", func() {
			msg := &testMsg{
				MapStrMsg: map[string]*testMsg{
					"a": {Num: 1, Str: "abc"},
					"b": {Num: 2, Str: "bcd"},
				},
			}
			testTrim([]string{"map_str_msg.*.num"}, msg)
			So(msg, ShouldResembleProto, &testMsg{
				MapStrMsg: map[string]*testMsg{
					"a": {Num: 1},
					"b": {Num: 2},
				},
			})
		})
		Convey("trim map (message value) with both star key and actual key", func() {
			msg := &testMsg{
				MapStrMsg: map[string]*testMsg{
					"a": {Num: 1, Str: "abc"},
					"b": {Num: 2, Str: "bcd"},
				},
			}
			testTrim([]string{"map_str_msg.*.num", "map_str_msg.a"}, msg)
			// expect keep "a" entirely and "b" partially
			So(msg, ShouldResembleProto, &testMsg{
				MapStrMsg: map[string]*testMsg{
					"a": {Num: 1, Str: "abc"},
					"b": {Num: 2},
				},
			})
		})
		Convey("No-op for empty mask trim", func() {
			m, msg := Mask{}, &testMsg{Num: 1}
			m.Trim(msg)
			So(msg, ShouldResembleProto, &testMsg{Num: 1})
		})
		Convey("Error when trim nil message", func() {
			m, err := FromFieldMask(&field_mask.FieldMask{Paths: []string{"str"}}, &testMsg{}, false, false)
			So(err, ShouldBeNil)
			var msg proto.Message
			err = m.Trim(msg)
			So(err, ShouldErrLike, "nil message")
		})
		Convey("Error when descriptor mismatch", func() {
			m, err := FromFieldMask(&field_mask.FieldMask{Paths: []string{"str"}}, &testMsg{}, false, false)
			So(err, ShouldBeNil)
			err = m.Trim(&testingpb.Simple{})
			So(err, ShouldErrLike, "expected message have descriptor: internal.testing.Full; got descriptor: internal.testing.Simple")
		})
	})
}

func TestIncludes(t *testing.T) {
	Convey("Test include", t, func() {
		testIncludes := func(maskPaths []string, path string, expectedIncl Inclusiveness) {
			m, err := FromFieldMask(&field_mask.FieldMask{Paths: maskPaths}, &testMsg{}, false, false)
			So(err, ShouldBeNil)
			actual, err := m.Includes(path)
			So(err, ShouldBeNil)
			So(actual, ShouldEqual, expectedIncl)
		}
		Convey("all", func() {
			testIncludes([]string{}, "str", IncludeEntirely)
		})
		Convey("entirely", func() {
			testIncludes([]string{"str"}, "str", IncludeEntirely)
		})
		Convey("entirely multiple levels", func() {
			testIncludes([]string{"msg.msg.str"}, "msg.msg.str", IncludeEntirely)
		})
		Convey("entirely star", func() {
			testIncludes([]string{"map_str_msg.*.str"}, "map_str_msg.xyz.str", IncludeEntirely)
		})
		Convey("partially", func() {
			testIncludes([]string{"msg.str"}, "msg", IncludePartially)
		})
		Convey("partially multiple levels", func() {
			testIncludes([]string{"msg.msg.str"}, "msg.msg", IncludePartially)
		})
		Convey("partially star", func() {
			testIncludes([]string{"map_str_msg.*.str"}, "map_str_msg.xyz", IncludePartially)
		})
		Convey("exclude", func() {
			testIncludes([]string{"str"}, "num", Exclude)
		})
		Convey("exclude multiple levels", func() {
			testIncludes([]string{"msg.str"}, "msg.num", Exclude)
		})
	})

	Convey("Test MustIncludes", t, func() {
		testMustIncludes := func(maskPaths []string, path string, expectedIncl Inclusiveness) {
			m, err := FromFieldMask(&field_mask.FieldMask{Paths: maskPaths}, &testMsg{}, false, false)
			So(err, ShouldBeNil)
			actual := m.MustIncludes(path)
			So(actual, ShouldEqual, expectedIncl)
		}

		Convey("works", func() {
			testMustIncludes([]string{}, "str", IncludeEntirely)
		})

		Convey("panics", func() {
			// expected delimiter: .; got @'
			So(func() { testMustIncludes([]string{}, "str@", IncludeEntirely) }, ShouldPanic)
		})
	})
}

func TestMerge(t *testing.T) {
	Convey("Test merge", t, func() {
		testMerge := func(maskPaths []string, src *testMsg, dest *testMsg) {
			m, err := FromFieldMask(&field_mask.FieldMask{Paths: maskPaths}, &testMsg{}, false, false)
			So(err, ShouldBeNil)
			So(m.Merge(src, dest), ShouldBeNil)
		}
		Convey("scalar field", func() {
			src := &testMsg{Num: 1}
			dest := &testMsg{Num: 2}
			testMerge([]string{"num"}, src, dest)
			So(dest, ShouldResembleProto, &testMsg{Num: 1})
		})
		Convey("repeated scalar field", func() {
			src := &testMsg{Nums: []int32{1, 2, 3}}
			dest := &testMsg{Nums: []int32{4, 5}}
			testMerge([]string{"nums"}, src, dest)
			So(dest, ShouldResembleProto, &testMsg{Nums: []int32{1, 2, 3}})
		})
		Convey("repeated message field", func() {
			src := &testMsg{
				Msgs: []*testMsg{
					{Num: 1},
					{Str: "abc"},
				},
			}
			dest := &testMsg{
				Msgs: []*testMsg{
					{Msg: &testMsg{}},
				},
			}
			testMerge([]string{"msgs"}, src, dest)
			So(dest, ShouldResembleProto, &testMsg{
				Msgs: []*testMsg{
					{Num: 1},
					{Str: "abc"},
				},
			})
		})
		Convey("entire submessage", func() {
			src := &testMsg{
				Msg: &testMsg{
					Num: 1,
					Str: "abc",
				},
			}
			dest := &testMsg{
				Msg: &testMsg{
					Num: 2,
					Str: "def",
				},
			}
			testMerge([]string{"msg"}, src, dest)
			So(dest, ShouldResembleProto, &testMsg{
				Msg: &testMsg{
					Num: 1,
					Str: "abc",
				},
			})
		})
		Convey("partial submessage", func() {
			src := &testMsg{
				Msg: &testMsg{
					Num: 1,
					Str: "abc",
				},
			}
			dest := &testMsg{
				Msg: &testMsg{
					Num: 2,
					Str: "def",
				},
			}
			testMerge([]string{"msg.num"}, src, dest)
			So(dest, ShouldResembleProto, &testMsg{
				Msg: &testMsg{
					Num: 1,
					Str: "def",
				},
			})
		})
		Convey("nil message", func() {
			src := &testMsg{
				Msg: nil,
			}
			Convey("last seg is message itself", func() {
				dest := &testMsg{
					Msg: &testMsg{
						Str: "def",
					},
				}
				testMerge([]string{"msg"}, src, dest)
				So(dest, ShouldResembleProto, &testMsg{})
			})
			Convey("last seg is field in message", func() {
				dest := &testMsg{
					Msg: &testMsg{
						Str: "def",
					},
				}
				testMerge([]string{"msg.num"}, src, dest)
				So(dest, ShouldResembleProto, &testMsg{
					Msg: &testMsg{
						Str: "def",
					},
				})
				testMerge([]string{"msg.str"}, src, dest)
				So(dest, ShouldResembleProto, &testMsg{
					Msg: &testMsg{},
				})
			})
			Convey("dest is also nil messages", func() {
				dest := &testMsg{
					Msg: nil,
				}
				testMerge([]string{"msg"}, src, dest)
				So(dest, ShouldResembleProto, &testMsg{})
				testMerge([]string{"msg.str"}, src, dest)
				So(dest, ShouldResembleProto, &testMsg{})
			})
		})
		Convey("map field ", func() {
			src := &testMsg{
				MapStrMsg: map[string]*testMsg{
					"a": {Num: 1},
					"b": {Num: 2},
				},
			}
			dest := &testMsg{
				MapStrMsg: map[string]*testMsg{
					"c": {Num: 1},
				},
			}
			testMerge([]string{"map_str_msg"}, src, dest)
			So(dest, ShouldResembleProto, &testMsg{
				MapStrMsg: map[string]*testMsg{
					"a": {Num: 1},
					"b": {Num: 2},
				},
			})
		})
		Convey("map field (dest map is nil)", func() {
			src := &testMsg{
				MapStrMsg: map[string]*testMsg{
					"a": {Num: 1},
					"b": {Num: 2},
				},
			}
			dest := &testMsg{
				MapStrMsg: nil,
			}
			testMerge([]string{"map_str_msg"}, src, dest)
			So(dest, ShouldResembleProto, &testMsg{
				MapStrMsg: map[string]*testMsg{
					"a": {Num: 1},
					"b": {Num: 2},
				},
			})
		})
		Convey("map field (value is nil message)", func() {
			src := &testMsg{
				MapStrMsg: map[string]*testMsg{
					"a": nil,
				},
			}
			dest := &testMsg{}
			testMerge([]string{"map_str_msg"}, src, dest)
			So(dest, ShouldResembleProto, &testMsg{
				MapStrMsg: map[string]*testMsg{
					"a": nil,
				},
			})
		})
		Convey("empty mask", func() {
			src := &testMsg{Num: 1}
			dest := &testMsg{Num: 2}
			m := &Mask{}
			So(m.Merge(src, dest), ShouldBeNil)
			So(dest, ShouldResembleProto, &testMsg{Num: 2})
		})
		Convey("multiple fields", func() {
			src := &testMsg{Num: 1, Strs: []string{"a", "b"}}
			dest := &testMsg{Num: 2, Strs: []string{"c"}}
			testMerge([]string{"num", "strs"}, src, dest)
			So(dest, ShouldResembleProto, &testMsg{Num: 1, Strs: []string{"a", "b"}})
		})
		Convey("Error when one of proto message is nil", func() {
			m, err := FromFieldMask(&field_mask.FieldMask{Paths: []string{"str"}}, &testMsg{}, false, false)
			So(err, ShouldBeNil)
			var src proto.Message
			So(m.Merge(src, &testMsg{}), ShouldErrLike, "src message: nil message")
		})
		Convey("Error when proto message descriptors does not match", func() {
			m, err := FromFieldMask(&field_mask.FieldMask{Paths: []string{"str"}}, &testMsg{}, false, false)
			So(err, ShouldBeNil)
			err = m.Merge(&testMsg{}, &testingpb.Simple{})
			So(err, ShouldErrLike, "dest message: expected message have descriptor: internal.testing.Full; got descriptor: internal.testing.Simple")
		})
	})
}

func TestSubmask(t *testing.T) {
	buildMask := func(paths ...string) *Mask {
		m, err := FromFieldMask(&field_mask.FieldMask{Paths: paths}, &testMsg{}, false, false)
		So(err, ShouldBeNil)
		return m
	}

	Convey("Test submask", t, func() {
		Convey("when path is partially included", func() {
			actual, err := buildMask("msg.msgs.*.str").Submask("msg")
			So(err, ShouldBeNil)
			assertMaskEqual(actual, buildMask("msgs.*.str"))
		})
		Convey("when path is entirely included", func() {
			actual, err := buildMask("msg").Submask("msg.msgs.*.msg")
			So(err, ShouldBeNil)
			assertMaskEqual(actual, &Mask{descriptor: testMsgDescriptor})
		})
		Convey("Error when path is excluded", func() {
			_, err := buildMask("msg.msg.str").Submask("str")
			So(err, ShouldErrLike, "the given path \"str\" is excluded from mask")
		})
		Convey("when path ends with star", func() {
			actual, err := buildMask("msgs.*.str").Submask("msgs.*")
			So(err, ShouldBeNil)
			assertMaskEqual(actual, buildMask("str"))
		})
	})

	Convey("Test MustSubmask", t, func() {
		m := buildMask("msg.msg.str")

		Convey("works", func() {
			assertMaskEqual(m.MustSubmask("msg.msg"), buildMask("str"))
		})

		Convey("panics", func() {
			// the given path "str" is excluded from mask
			So(func() { m.MustSubmask("str") }, ShouldPanic)
			// expected delimiter: .; got @'
			So(func() { m.MustSubmask("str@") }, ShouldPanic)
		})
	})
}

// TODO(yiwzhang): ShouldResemble will hit infinite loop when comparing
// descriptor. Comparing the full name of message as a temporary workaround
func assertMaskEqual(actual *Mask, expect *Mask) {
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
