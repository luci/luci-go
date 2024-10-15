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
	"github.com/google/go-cmp/cmp"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.chromium.org/luci/common/proto/internal/testingpb"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/registry"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func init() {
	registry.RegisterCmpOption(cmp.AllowUnexported(Mask{}))
}

type testMsg = testingpb.Full

var testMsgDescriptor = proto.MessageReflect(&testMsg{}).Descriptor()

func TestNormalizePath(t *testing.T) {
	ftt.Run("Retrun empty paths when given empty paths", t, func(t *ftt.Test) {
		assert.Loosely(t, normalizePaths([]path{}), should.Resemble([]path{}))
	})
	ftt.Run("Remove all deduplicate paths", t, func(t *ftt.Test) {
		assert.Loosely(t, normalizePaths([]path{
			{"a", "b"},
			{"a", "b"},
			{"a", "b"},
		}), should.Resemble([]path{
			{"a", "b"},
		}))
	})
	ftt.Run("Remove all redundant paths and return sorted", t, func(t *ftt.Test) {
		assert.Loosely(t, normalizePaths([]path{
			{"b", "z"},
			{"b", "c", "d"},
			{"b", "c"},
			{"a"},
		}), should.Resemble([]path{
			{"a"},
			{"b", "c"},
			{"b", "z"},
		}))
	})
}

func TestFromFieldMask(t *testing.T) {
	ftt.Run("From", t, func(t *ftt.Test) {
		parse := func(paths []string, isUpdateMask bool) (*Mask, error) {
			return FromFieldMask(&field_mask.FieldMask{Paths: paths}, &testMsg{}, false, isUpdateMask)
		}

		t.Run("empty field mask", func(t *ftt.Test) {
			actual, err := parse([]string{}, false)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, actual, should.Match(&Mask{
				descriptor: testMsgDescriptor,
			}))
		})

		t.Run("field mask with scalar and message fields", func(t *ftt.Test) {
			actual, err := parse([]string{"str", "num", "msg.num"}, false)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, actual, should.Match(&Mask{
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
			}))
		})
		t.Run("field mask with map field", func(t *ftt.Test) {
			actual, err := parse([]string{"map_str_msg.some_key.str", "map_str_num.another_key"}, false)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, actual, should.Match(&Mask{
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
			}))
		})
		t.Run("field mask with repeated field", func(t *ftt.Test) {
			actual, err := parse([]string{"nums", "msgs.*.str"}, false)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, actual, should.Match(&Mask{
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
			}))
		})
		t.Run("update mask", func(t *ftt.Test) {
			_, err := parse([]string{"msgs.*.str"}, true)
			assert.Loosely(t, err, should.ErrLike("update mask allows a repeated field only at the last position; field: msgs is not last"))
			_, err = parse([]string{"map_str_msg.*.str"}, true)
			assert.Loosely(t, err, should.ErrLike("update mask allows a repeated field only at the last position; field: map_str_msg is not last"))
			_, err = parse([]string{"map_str_num.some_key"}, true)
			assert.Loosely(t, err, should.BeNil)
		})
	})
}
func TestTrim(t *testing.T) {
	ftt.Run("Test", t, func(t *ftt.Test) {
		testTrim := func(maskPaths []string, msg proto.Message) {
			m, err := FromFieldMask(&field_mask.FieldMask{Paths: maskPaths}, &testMsg{}, false, false)
			assert.Loosely(t, err, should.BeNil)
			err = m.Trim(msg)
			assert.Loosely(t, err, should.BeNil)
		}
		t.Run("trim scalar field", func(t *ftt.Test) {
			msg := &testMsg{Num: 1}
			testTrim([]string{"str"}, msg)
			assert.Loosely(t, msg, should.Resemble(&testMsg{}))
		})
		t.Run("keep scalar field", func(t *ftt.Test) {
			msg := &testMsg{Num: 1}
			testTrim([]string{"num"}, msg)
			assert.Loosely(t, msg, should.Resemble(&testMsg{Num: 1}))
		})
		t.Run("trim repeated scalar field", func(t *ftt.Test) {
			msg := &testMsg{Nums: []int32{1, 2}}
			testTrim([]string{"str"}, msg)
			assert.Loosely(t, msg, should.Resemble(&testMsg{}))
		})
		t.Run("keep repeated scalar field", func(t *ftt.Test) {
			msg := &testMsg{Nums: []int32{1, 2}}
			testTrim([]string{"nums"}, msg)
			assert.Loosely(t, msg, should.Resemble(&testMsg{Nums: []int32{1, 2}}))
		})
		t.Run("trim submessage", func(t *ftt.Test) {
			msg := &testMsg{
				Msg: &testMsg{
					Num: 1,
				},
			}
			testTrim([]string{"str"}, msg)
			assert.Loosely(t, msg, should.Resemble(&testMsg{}))
		})
		t.Run("keep submessage entirely", func(t *ftt.Test) {
			msg := &testMsg{
				Msg: &testMsg{
					Num: 1,
					Str: "abc",
				},
			}
			testTrim([]string{"msg"}, msg)
			assert.Loosely(t, msg, should.Resemble(&testMsg{
				Msg: &testMsg{
					Num: 1,
					Str: "abc",
				},
			}))
		})
		t.Run("keep submessage partially", func(t *ftt.Test) {
			msg := &testMsg{
				Msg: &testMsg{
					Num: 1,
					Str: "abc",
				},
			}
			testTrim([]string{"msg.str"}, msg)
			assert.Loosely(t, msg, should.Resemble(&testMsg{
				Msg: &testMsg{
					Str: "abc",
				},
			}))
		})
		t.Run("trim repeated message field", func(t *ftt.Test) {
			msg := &testMsg{
				Msgs: []*testMsg{
					{Num: 1},
					{Num: 2},
				},
			}
			testTrim([]string{"str"}, msg)
			assert.Loosely(t, msg, should.Resemble(&testMsg{}))
		})
		t.Run("keep subfield of repeated message field entirely", func(t *ftt.Test) {
			msg := &testMsg{
				Msgs: []*testMsg{
					{Num: 1, Str: "abc"},
					{Num: 2, Str: "bcd"},
				},
			}
			testTrim([]string{"msgs"}, msg)
			assert.Loosely(t, msg, should.Resemble(&testMsg{
				Msgs: []*testMsg{
					{Num: 1, Str: "abc"},
					{Num: 2, Str: "bcd"},
				},
			}))
		})
		t.Run("keep subfield of repeated message field partially", func(t *ftt.Test) {
			msg := &testMsg{
				Msgs: []*testMsg{
					{Num: 1, Str: "abc"},
					{Num: 2, Str: "bcd"},
				},
			}
			testTrim([]string{"msgs.*.str"}, msg)
			assert.Loosely(t, msg, should.Resemble(&testMsg{
				Msgs: []*testMsg{
					{Str: "abc"},
					{Str: "bcd"},
				},
			}))
		})
		t.Run("trim map field", func(t *ftt.Test) {
			msg := &testMsg{
				MapStrNum: map[string]int32{"a": 1},
			}
			testTrim([]string{"str"}, msg)
			assert.Loosely(t, msg, should.Resemble(&testMsg{}))
		})
		t.Run("trim map (scalar value)", func(t *ftt.Test) {
			msg := &testMsg{
				MapStrNum: map[string]int32{
					"a": 1,
					"b": 2,
				},
			}
			testTrim([]string{"map_str_num.a"}, msg)
			assert.Loosely(t, msg, should.Resemble(&testMsg{
				MapStrNum: map[string]int32{
					"a": 1,
				},
			}))
		})
		t.Run("trim map (message value) ", func(t *ftt.Test) {
			msg := &testMsg{
				MapStrMsg: map[string]*testMsg{
					"a": {Num: 1},
					"b": {Num: 2},
				},
			}
			testTrim([]string{"map_str_msg.a"}, msg)
			assert.Loosely(t, msg, should.Resemble(&testMsg{
				MapStrMsg: map[string]*testMsg{
					"a": {Num: 1},
				},
			}))
		})
		t.Run("trim map (message value) and keep message value partially", func(t *ftt.Test) {
			msg := &testMsg{
				MapStrMsg: map[string]*testMsg{
					"a": {Num: 1, Str: "abc"},
					"b": {Num: 2, Str: "bcd"},
				},
			}
			testTrim([]string{"map_str_msg.a.num"}, msg)
			assert.Loosely(t, msg, should.Resemble(&testMsg{
				MapStrMsg: map[string]*testMsg{
					"a": {Num: 1},
				},
			}))
		})
		t.Run("trim map (message value) with star key and keep message value partially", func(t *ftt.Test) {
			msg := &testMsg{
				MapStrMsg: map[string]*testMsg{
					"a": {Num: 1, Str: "abc"},
					"b": {Num: 2, Str: "bcd"},
				},
			}
			testTrim([]string{"map_str_msg.*.num"}, msg)
			assert.Loosely(t, msg, should.Resemble(&testMsg{
				MapStrMsg: map[string]*testMsg{
					"a": {Num: 1},
					"b": {Num: 2},
				},
			}))
		})
		t.Run("trim map (message value) with both star key and actual key", func(t *ftt.Test) {
			msg := &testMsg{
				MapStrMsg: map[string]*testMsg{
					"a": {Num: 1, Str: "abc"},
					"b": {Num: 2, Str: "bcd"},
				},
			}
			testTrim([]string{"map_str_msg.*.num", "map_str_msg.a"}, msg)
			// expect keep "a" entirely and "b" partially
			assert.Loosely(t, msg, should.Resemble(&testMsg{
				MapStrMsg: map[string]*testMsg{
					"a": {Num: 1, Str: "abc"},
					"b": {Num: 2},
				},
			}))
		})
		t.Run("No-op for empty mask trim", func(t *ftt.Test) {
			m, msg := Mask{}, &testMsg{Num: 1}
			m.Trim(msg)
			assert.Loosely(t, msg, should.Resemble(&testMsg{Num: 1}))
		})
		t.Run("Error when trim nil message", func(t *ftt.Test) {
			m, err := FromFieldMask(&field_mask.FieldMask{Paths: []string{"str"}}, &testMsg{}, false, false)
			assert.Loosely(t, err, should.BeNil)
			var msg proto.Message
			err = m.Trim(msg)
			assert.Loosely(t, err, should.ErrLike("nil message"))
		})
		t.Run("Error when descriptor mismatch", func(t *ftt.Test) {
			m, err := FromFieldMask(&field_mask.FieldMask{Paths: []string{"str"}}, &testMsg{}, false, false)
			assert.Loosely(t, err, should.BeNil)
			err = m.Trim(&testingpb.Simple{})
			assert.Loosely(t, err, should.ErrLike("expected message have descriptor: internal.testing.Full; got descriptor: internal.testing.Simple"))
		})
	})
}

func TestIncludes(t *testing.T) {
	ftt.Run("Test include", t, func(t *ftt.Test) {
		testIncludes := func(maskPaths []string, path string, expectedIncl Inclusiveness) {
			m, err := FromFieldMask(&field_mask.FieldMask{Paths: maskPaths}, &testMsg{}, false, false)
			assert.Loosely(t, err, should.BeNil)
			actual, err := m.Includes(path)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.Equal(expectedIncl))
		}
		t.Run("all", func(t *ftt.Test) {
			testIncludes([]string{}, "str", IncludeEntirely)
		})
		t.Run("entirely", func(t *ftt.Test) {
			testIncludes([]string{"str"}, "str", IncludeEntirely)
		})
		t.Run("entirely multiple levels", func(t *ftt.Test) {
			testIncludes([]string{"msg.msg.str"}, "msg.msg.str", IncludeEntirely)
		})
		t.Run("entirely star", func(t *ftt.Test) {
			testIncludes([]string{"map_str_msg.*.str"}, "map_str_msg.xyz.str", IncludeEntirely)
		})
		t.Run("partially", func(t *ftt.Test) {
			testIncludes([]string{"msg.str"}, "msg", IncludePartially)
		})
		t.Run("partially multiple levels", func(t *ftt.Test) {
			testIncludes([]string{"msg.msg.str"}, "msg.msg", IncludePartially)
		})
		t.Run("partially star", func(t *ftt.Test) {
			testIncludes([]string{"map_str_msg.*.str"}, "map_str_msg.xyz", IncludePartially)
		})
		t.Run("exclude", func(t *ftt.Test) {
			testIncludes([]string{"str"}, "num", Exclude)
		})
		t.Run("exclude multiple levels", func(t *ftt.Test) {
			testIncludes([]string{"msg.str"}, "msg.num", Exclude)
		})
	})

	ftt.Run("Test MustIncludes", t, func(t *ftt.Test) {
		testMustIncludes := func(maskPaths []string, path string, expectedIncl Inclusiveness) {
			m, err := FromFieldMask(&field_mask.FieldMask{Paths: maskPaths}, &testMsg{}, false, false)
			assert.Loosely(t, err, should.BeNil)
			actual := m.MustIncludes(path)
			assert.Loosely(t, actual, should.Equal(expectedIncl))
		}

		t.Run("works", func(t *ftt.Test) {
			testMustIncludes([]string{}, "str", IncludeEntirely)
		})

		t.Run("panics", func(t *ftt.Test) {
			// expected delimiter: .; got @'
			assert.Loosely(t, func() { testMustIncludes([]string{}, "str@", IncludeEntirely) }, should.Panic)
		})
	})
}

func TestMerge(t *testing.T) {
	ftt.Run("Test merge", t, func(t *ftt.Test) {
		testMerge := func(maskPaths []string, src *testMsg, dest *testMsg) {
			m, err := FromFieldMask(&field_mask.FieldMask{Paths: maskPaths}, &testMsg{}, false, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, m.Merge(src, dest), should.BeNil)
		}
		t.Run("scalar field", func(t *ftt.Test) {
			src := &testMsg{Num: 1}
			dest := &testMsg{Num: 2}
			testMerge([]string{"num"}, src, dest)
			assert.Loosely(t, dest, should.Resemble(&testMsg{Num: 1}))
		})
		t.Run("repeated scalar field", func(t *ftt.Test) {
			src := &testMsg{Nums: []int32{1, 2, 3}}
			dest := &testMsg{Nums: []int32{4, 5}}
			testMerge([]string{"nums"}, src, dest)
			assert.Loosely(t, dest, should.Resemble(&testMsg{Nums: []int32{1, 2, 3}}))
		})
		t.Run("repeated message field", func(t *ftt.Test) {
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
			assert.Loosely(t, dest, should.Resemble(&testMsg{
				Msgs: []*testMsg{
					{Num: 1},
					{Str: "abc"},
				},
			}))
		})
		t.Run("entire submessage", func(t *ftt.Test) {
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
			assert.Loosely(t, dest, should.Resemble(&testMsg{
				Msg: &testMsg{
					Num: 1,
					Str: "abc",
				},
			}))
		})
		t.Run("partial submessage", func(t *ftt.Test) {
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
			assert.Loosely(t, dest, should.Resemble(&testMsg{
				Msg: &testMsg{
					Num: 1,
					Str: "def",
				},
			}))
		})
		t.Run("nil message", func(t *ftt.Test) {
			src := &testMsg{
				Msg: nil,
			}
			t.Run("last seg is message itself", func(t *ftt.Test) {
				dest := &testMsg{
					Msg: &testMsg{
						Str: "def",
					},
				}
				testMerge([]string{"msg"}, src, dest)
				assert.Loosely(t, dest, should.Resemble(&testMsg{}))
			})
			t.Run("last seg is field in message", func(t *ftt.Test) {
				dest := &testMsg{
					Msg: &testMsg{
						Str: "def",
					},
				}
				testMerge([]string{"msg.num"}, src, dest)
				assert.Loosely(t, dest, should.Resemble(&testMsg{
					Msg: &testMsg{
						Str: "def",
					},
				}))
				testMerge([]string{"msg.str"}, src, dest)
				assert.Loosely(t, dest, should.Resemble(&testMsg{
					Msg: &testMsg{},
				}))
			})
			t.Run("dest is also nil messages", func(t *ftt.Test) {
				dest := &testMsg{
					Msg: nil,
				}
				testMerge([]string{"msg"}, src, dest)
				assert.Loosely(t, dest, should.Resemble(&testMsg{}))
				testMerge([]string{"msg.str"}, src, dest)
				assert.Loosely(t, dest, should.Resemble(&testMsg{}))
			})
		})
		t.Run("map field ", func(t *ftt.Test) {
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
			assert.Loosely(t, dest, should.Resemble(&testMsg{
				MapStrMsg: map[string]*testMsg{
					"a": {Num: 1},
					"b": {Num: 2},
				},
			}))
		})
		t.Run("map field (dest map is nil)", func(t *ftt.Test) {
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
			assert.Loosely(t, dest, should.Resemble(&testMsg{
				MapStrMsg: map[string]*testMsg{
					"a": {Num: 1},
					"b": {Num: 2},
				},
			}))
		})
		t.Run("map field (value is nil message)", func(t *ftt.Test) {
			src := &testMsg{
				MapStrMsg: map[string]*testMsg{
					"a": nil,
				},
			}
			dest := &testMsg{}
			testMerge([]string{"map_str_msg"}, src, dest)
			assert.Loosely(t, dest, should.Resemble(&testMsg{
				MapStrMsg: map[string]*testMsg{
					"a": nil,
				},
			}))
		})
		t.Run("empty mask", func(t *ftt.Test) {
			src := &testMsg{Num: 1}
			dest := &testMsg{Num: 2}
			m := &Mask{}
			assert.Loosely(t, m.Merge(src, dest), should.BeNil)
			assert.Loosely(t, dest, should.Resemble(&testMsg{Num: 2}))
		})
		t.Run("multiple fields", func(t *ftt.Test) {
			src := &testMsg{Num: 1, Strs: []string{"a", "b"}}
			dest := &testMsg{Num: 2, Strs: []string{"c"}}
			testMerge([]string{"num", "strs"}, src, dest)
			assert.Loosely(t, dest, should.Resemble(&testMsg{Num: 1, Strs: []string{"a", "b"}}))
		})
		t.Run("Error when one of proto message is nil", func(t *ftt.Test) {
			m, err := FromFieldMask(&field_mask.FieldMask{Paths: []string{"str"}}, &testMsg{}, false, false)
			assert.Loosely(t, err, should.BeNil)
			var src proto.Message
			assert.Loosely(t, m.Merge(src, &testMsg{}), should.ErrLike("src message: nil message"))
		})
		t.Run("Error when proto message descriptors does not match", func(t *ftt.Test) {
			m, err := FromFieldMask(&field_mask.FieldMask{Paths: []string{"str"}}, &testMsg{}, false, false)
			assert.Loosely(t, err, should.BeNil)
			err = m.Merge(&testMsg{}, &testingpb.Simple{})
			assert.Loosely(t, err, should.ErrLike("dest message: expected message have descriptor: internal.testing.Full; got descriptor: internal.testing.Simple"))
		})
	})
}

func TestSubmask(t *testing.T) {
	buildMask := func(paths ...string) *Mask {
		m, err := FromFieldMask(&field_mask.FieldMask{Paths: paths}, &testMsg{}, false, false)
		assert.Loosely(t, err, should.BeNil)
		return m
	}

	ftt.Run("Test submask", t, func(t *ftt.Test) {
		t.Run("when path is partially included", func(t *ftt.Test) {
			actual, err := buildMask("msg.msgs.*.str").Submask("msg")
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, actual, should.Match(buildMask("msgs.*.str")))
		})
		t.Run("when path is entirely included", func(t *ftt.Test) {
			actual, err := buildMask("msg").Submask("msg.msgs.*.msg")
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, actual, should.Match(&Mask{descriptor: testMsgDescriptor}))
		})
		t.Run("Error when path is excluded", func(t *ftt.Test) {
			_, err := buildMask("msg.msg.str").Submask("str")
			assert.Loosely(t, err, should.ErrLike("the given path \"str\" is excluded from mask"))
		})
		t.Run("when path ends with star", func(t *ftt.Test) {
			actual, err := buildMask("msgs.*.str").Submask("msgs.*")
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, actual, should.Match(buildMask("str")))
		})
	})

	ftt.Run("Test MustSubmask", t, func(t *ftt.Test) {
		m := buildMask("msg.msg.str")

		t.Run("works", func(t *ftt.Test) {
			assert.That(t, m.MustSubmask("msg.msg"), should.Match(buildMask("str")))
		})

		t.Run("panics", func(t *ftt.Test) {
			// the given path "str" is excluded from mask
			assert.Loosely(t, func() { m.MustSubmask("str") }, should.Panic)
			// expected delimiter: .; got @'
			assert.Loosely(t, func() { m.MustSubmask("str@") }, should.Panic)
		})
	})
}
