// Copyright 2016 The LUCI Authors.
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

package flagpb

import (
	"os"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"go.chromium.org/luci/common/proto/google/descutil"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestUnmarshal(t *testing.T) {
	t.Parallel()

	ftt.Run("Unmarshal", t, func(t *ftt.Test) {
		descFileBytes, err := os.ReadFile("unmarshal_test.desc")
		assert.Loosely(t, err, should.BeNil)

		var desc descriptorpb.FileDescriptorSet
		err = proto.Unmarshal(descFileBytes, &desc)
		assert.Loosely(t, err, should.BeNil)

		resolver := NewResolver(&desc)

		resolveMsg := func(name string) *descriptorpb.DescriptorProto {
			_, obj, _ := descutil.Resolve(&desc, "flagpb."+name)
			assert.Loosely(t, obj, should.NotBeNil)
			return obj.(*descriptorpb.DescriptorProto)
		}

		unmarshalOK := func(typeName string, args ...string) map[string]any {
			msg, err := UnmarshalUntyped(args, resolveMsg(typeName), resolver)
			assert.Loosely(t, err, should.BeNil)
			return msg
		}

		unmarshalErr := func(typeName string, args ...string) error {
			msg, err := UnmarshalUntyped(args, resolveMsg(typeName), resolver)
			assert.Loosely(t, msg, should.BeNil)
			return err
		}

		t.Run("empty", func(t *ftt.Test) {
			assert.Loosely(t, unmarshalOK("M1"), should.Match(msg()))
		})
		t.Run("non-flag", func(t *ftt.Test) {
			assert.Loosely(t, unmarshalErr("M1", "abc"), should.ErrLike(`abc: a flag was expected`))
		})
		t.Run("string", func(t *ftt.Test) {
			t.Run("next arg", func(t *ftt.Test) {
				assert.Loosely(t, unmarshalOK("M1", "-s", "x"), should.Match(msg("s", "x")))
			})
			t.Run("equals sign", func(t *ftt.Test) {
				assert.Loosely(t, unmarshalOK("M1", "-s=x"), should.Match(msg("s", "x")))
			})
		})
		t.Run("int32", func(t *ftt.Test) {
			t.Run("next arg", func(t *ftt.Test) {
				assert.Loosely(t, unmarshalOK("M1", "-i", "1"), should.Match(msg("i", int32(1))))
			})
			t.Run("equals sign", func(t *ftt.Test) {
				assert.Loosely(t, unmarshalOK("M1", "-i=1"), should.Match(msg(
					"i", int32(1),
				)))
			})
			t.Run("error", func(t *ftt.Test) {
				assert.Loosely(t, unmarshalErr("M1", "-i", "abc"), should.ErrLike("invalid syntax"))
				assert.Loosely(t, unmarshalErr("M1", "-i=abc"), should.ErrLike("invalid syntax"))
			})
		})
		t.Run("enum", func(t *ftt.Test) {
			t.Run("by name", func(t *ftt.Test) {
				assert.Loosely(t, unmarshalOK("M2", "-e", "V0"), should.Match(msg("e", int32(0))))
			})
			t.Run("error", func(t *ftt.Test) {
				assert.Loosely(t, unmarshalErr("M2", "-e", "abc"), should.ErrLike(`invalid value "abc" for enum E`))
			})
			t.Run("by value", func(t *ftt.Test) {
				assert.Loosely(t, unmarshalOK("M2", "-e", "0"), should.Match(msg("e", int32(0))))
			})
		})
		t.Run("bool", func(t *ftt.Test) {
			t.Run("without value", func(t *ftt.Test) {
				assert.Loosely(t, unmarshalOK("M1", "-b"), should.Match(msg("b", true)))

				assert.Loosely(t, unmarshalOK("M1", "-b", "-s", "x"), should.Match(msg(
					"b", true,
					"s", "x",
				)))
			})
			t.Run("without value, repeated", func(t *ftt.Test) {
				assert.Loosely(t, unmarshalOK("M1", "-rb=false", "-rb"), should.Match(msg("rb", repeated(false, true))))
			})
			t.Run("with value", func(t *ftt.Test) {
				assert.Loosely(t, unmarshalOK("M1", "-b=true"), should.Match(msg("b", true)))
				assert.Loosely(t, unmarshalOK("M1", "-b=false"), should.Match(msg("b", false)))
			})
		})
		t.Run("bytes", func(t *ftt.Test) {
			t.Run("next arg", func(t *ftt.Test) {
				assert.Loosely(t, unmarshalOK("M1", "-bb", "6869"), should.Match(msg("bb", []byte("hi"))))
			})
			t.Run("equals sign", func(t *ftt.Test) {
				assert.Loosely(t, unmarshalOK("M1", "-bb=6869"), should.Match(msg("bb", []byte("hi"))))
			})
			t.Run("error", func(t *ftt.Test) {
				assert.Loosely(t, unmarshalErr("M1", "-bb", "xx"), should.ErrLike("invalid byte: U+0078 'x'"))
			})
		})

		t.Run("many dashes", func(t *ftt.Test) {
			t.Run("2", func(t *ftt.Test) {
				assert.Loosely(t, unmarshalOK("M1", "--s", "x"), should.Match(msg("s", "x")))
			})
			t.Run("3", func(t *ftt.Test) {
				assert.Loosely(t, unmarshalErr("M1", "---s", "x"), should.ErrLike("---s: bad flag syntax"))
			})
		})

		t.Run("field not found", func(t *ftt.Test) {
			assert.Loosely(t, unmarshalErr("M2", "-abc", "abc"), should.ErrLike(`-abc: field abc not found in message M2`))
		})
		t.Run("value not specified", func(t *ftt.Test) {
			assert.Loosely(t, unmarshalErr("M1", "-s"), should.ErrLike(`value was expected`))
		})

		t.Run("message", func(t *ftt.Test) {
			t.Run("level 1", func(t *ftt.Test) {
				assert.Loosely(t, unmarshalOK("M2", "-m1.s", "x"), should.Match(msg(
					"m1", msg("s", "x"),
				)))
				assert.Loosely(t, unmarshalOK("M2", "-m1.s", "x", "-m1.b"), should.Match(msg(
					"m1", msg(
						"s", "x",
						"b", true,
					),
				)))
			})
			t.Run("level 2", func(t *ftt.Test) {
				assert.Loosely(t, unmarshalOK("M3", "-m2.m1.s", "x"), should.Match(msg(
					"m2", msg(
						"m1", msg("s", "x"),
					),
				)))
			})
			t.Run("not found", func(t *ftt.Test) {
				assert.Loosely(t, unmarshalErr("M2", "-abc.s", "x"), should.ErrLike(`field "abc" not found in message M2`))
			})
			t.Run("non-msg subfield", func(t *ftt.Test) {
				assert.Loosely(t, unmarshalErr("M1", "-s.dummy", "x"), should.ErrLike("field s is not a message"))
			})
			t.Run("message value", func(t *ftt.Test) {
				const err = "M2.m1 is a message field. Specify its field values, not the message itself"
				assert.Loosely(t, unmarshalErr("M2", "-m1", "x"), should.ErrLike(err))
				assert.Loosely(t, unmarshalErr("M2", "-m1=x"), should.ErrLike(err))
			})
		})

		t.Run("string and int32", func(t *ftt.Test) {
			assert.Loosely(t, unmarshalOK("M1", "-s", "x", "-i", "1"), should.Match(msg(
				"s", "x",
				"i", int32(1),
			)))
		})

		t.Run("repeated", func(t *ftt.Test) {
			t.Run("int32", func(t *ftt.Test) {
				assert.Loosely(t, unmarshalOK("M1", "-ri", "1", "-ri", "2"), should.Match(msg(
					"ri", repeated(int32(1), int32(2)),
				)))
			})
			t.Run("submessage string", func(t *ftt.Test) {
				t.Run("works", func(t *ftt.Test) {
					assert.Loosely(t, unmarshalOK("M3", "-m1.s", "x", "-m1", "-m1.s", "y"), should.Match(msg(
						"m1", repeated(
							msg("s", "x"),
							msg("s", "y"),
						),
					)))
				})
				t.Run("reports meaningful error", func(t *ftt.Test) {
					err := unmarshalErr("M3", "-m1.s", "x", "-m1.s", "y")
					assert.Loosely(t, err, should.ErrLike(`-m1.s: value is already set`))
					assert.Loosely(t, err, should.ErrLike(`insert -m1`))
				})
			})
		})

		t.Run("map", func(t *ftt.Test) {
			t.Run("map<string, string>", func(t *ftt.Test) {
				assert.Loosely(t, unmarshalOK("MapContainer", "-ss.x", "a", "-ss.y", "b"), should.Match(msg(
					"ss", msg("x", "a", "y", "b"),
				)))
			})
			t.Run("map<int32, int32>", func(t *ftt.Test) {
				assert.Loosely(t, unmarshalOK("MapContainer", "-ii.1", "10", "-ii.2", "20"), should.Match(msg(
					"ii", msg("1", int32(10), "2", int32(20)),
				)))
			})
			t.Run("map<string, M1>", func(t *ftt.Test) {
				assert.Loosely(t, unmarshalOK("MapContainer", "-sm1.x.s", "a", "-sm1.y.s", "b"), should.Match(msg(
					"sm1", msg(
						"x", msg("s", "a"),
						"y", msg("s", "b"),
					),
				)))
			})
		})
	})
}
