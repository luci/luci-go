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

package lucicfg

import (
	"os"
	"testing"

	"go.starlark.net/starlark"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"go.chromium.org/luci/common/proto/textpb"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/starlark/starlarkproto"

	"go.chromium.org/luci/lucicfg/internal"
)

// testMessageType represents testproto.Msg from misc/support/test.proto as
// loaded through starlarkproto Loader.
//
// Used by testMessage and testMessageProto.
var testMessageType *starlarkproto.MessageType

func init() {
	// See testdata/gen.go for where this file is generated.
	blob, err := os.ReadFile("testdata/core/misc/support/test_descpb.bin")
	if err != nil {
		panic(err)
	}
	dspb := &descriptorpb.FileDescriptorSet{}
	if err := proto.Unmarshal(blob, dspb); err != nil {
		panic(err)
	}
	ds, err := starlarkproto.NewDescriptorSet("test", dspb.GetFile(), []*starlarkproto.DescriptorSet{
		luciTypesDescSet, // for "go.chromium.org/luci/common/proto/options.proto"
	})
	if err != nil {
		panic(err)
	}
	testProtoLoader := starlarkproto.NewLoader()
	if err := testProtoLoader.AddDescriptorSet(ds); err != nil {
		panic(err)
	}
	testproto, err := testProtoLoader.Module("misc/support/test.proto")
	if err != nil {
		panic(err)
	}
	msgT, err := testproto.Attr("Msg")
	if err != nil {
		panic(err)
	}
	testMessageType = msgT.(*starlarkproto.MessageType)
}

// testMessage returns new testproto.Msg as a Starlark value to be used from
// tests (grabs it via testProtoLoader).
func testMessage(i int, f float64) *starlarkproto.Message {
	msg := testMessageType.Message()
	if err := msg.SetField("i", starlark.MakeInt(i)); err != nil {
		panic(err)
	}
	if err := msg.SetField("f", starlark.Float(f)); err != nil {
		panic(err)
	}
	return msg
}

// testMessageProto returns new testproto.Msg as proto.Message, deserializing
// it from a text proto.
func testMessageProto(body string) proto.Message {
	msg, err := starlarkproto.FromTextPB(testMessageType, []byte(body), false)
	if err != nil {
		panic(err)
	}
	return msg.ToProto()
}

func TestProtos(t *testing.T) {
	t.Parallel()

	// Note: testMessage() is used by other tests. This test verifies it works
	// at all.
	ftt.Run("testMessage works", t, func(t *ftt.Test) {
		i, err := testMessage(123, 0).Attr("i")
		assert.Loosely(t, err, should.BeNil)
		asInt, err := starlark.AsInt32(i)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, asInt, should.Equal(123))
	})

	ftt.Run("testMessageProto works", t, func(t *ftt.Test) {
		msg := testMessageProto("i: 456")
		blob, err := textpb.Marshal(msg)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, string(blob), should.Equal("i: 456\n"))
	})

	ftt.Run("Doc URL works", t, func(t *ftt.Test) {
		name, doc := protoMessageDoc(testMessage(123, 0))
		assert.Loosely(t, name, should.Equal("Msg"))
		assert.Loosely(t, doc, should.Equal("https://example.com/proto-doc")) // see misc/support/test.proto
	})

	ftt.Run("SemanticProtoEqual: true", t, func(t *ftt.Test) {
		msg1 := testMessageProto(`
			i: 123
			nested: {
				s: "aaa"
				ignore: "ignore 1"
			}
			ignore_scalar: "ignore 1"
			ignore_rep: "ignore 1"
			ignore_rep: "ignore 1"
			ignore_nested: {
				s: "ignore 1"
			}
		`)
		msg2 := testMessageProto(`
			i: 123
			nested: {
				s: "aaa"
				ignore: "ignore 2"
			}
			ignore_scalar: "ignore 2"
			ignore_rep: "ignore 2"
			ignore_rep: "ignore 2"
			ignore_nested: {
				s: "ignore 2"
			}
		`)
		assert.Loosely(t, internal.SemanticProtoEqual(msg1, msg2), should.BeTrue)
	})

	ftt.Run("SemanticProtoEqual: false", t, func(t *ftt.Test) {
		msg1 := testMessageProto(`
			i: 123
			nested: {
				s: "aaa"
				ignore: "ignore 1"
			}
			ignore_scalar: "ignore 1"
			ignore_rep: "ignore 1"
			ignore_rep: "ignore 1"
			ignore_nested: {
				s: "ignore 1"
			}
		`)
		msg2 := testMessageProto(`
			i: 123
			nested: {
				s: "bbb"
				ignore: "ignore 2"
			}
			ignore_scalar: "ignore 2"
			ignore_rep: "ignore 2"
			ignore_rep: "ignore 2"
			ignore_nested: {
				s: "ignore 2"
			}
		`)
		assert.Loosely(t, internal.SemanticProtoEqual(msg1, msg2), should.BeFalse)
	})
}
