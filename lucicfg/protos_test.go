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
	"fmt"
	"io/ioutil"
	"testing"

	"go.starlark.net/starlark"

	"go.chromium.org/luci/starlark/protohacks"
	"go.chromium.org/luci/starlark/starlarkproto"

	. "github.com/smartystreets/goconvey/convey"
)

// testProtoLoader is a proto.Loader populated with misc/support/test.proto
// descriptor and its dependencies. Used by testMessage().
var testProtoLoader *starlarkproto.Loader

func init() {
	// See testdata/gen.go for where this file is generated.
	blob, err := ioutil.ReadFile("testdata/misc/support/test_descpb.bin")
	if err != nil {
		panic(err)
	}
	dspb, err := protohacks.UnmarshalFileDescriptorSet(blob)
	if err != nil {
		panic(err)
	}
	ds, err := starlarkproto.NewDescriptorSet("test", dspb.GetFile(), []*starlarkproto.DescriptorSet{
		luciTypesDescSet, // for "go.chromium.org/luci/common/proto/options.proto"
	})
	if err != nil {
		panic(err)
	}
	testProtoLoader = starlarkproto.NewLoader()
	if err := testProtoLoader.AddDescriptorSet(ds); err != nil {
		panic(err)
	}
}

// testMessage returns new testproto.Msg as a Starlark value to be used from
// tests (grabs it via testProtoLoader).
func testMessage(i int) *starlarkproto.Message {
	testproto, err := testProtoLoader.Module("misc/support/test.proto")
	if err != nil {
		panic(err)
	}
	msgT, err := testproto.Attr("Msg")
	if err != nil {
		panic(err)
	}
	msg := msgT.(*starlarkproto.MessageType).Message()
	if err := msg.SetField("i", starlark.MakeInt(i)); err != nil {
		panic(err)
	}
	return msg
}

func TestProtos(t *testing.T) {
	t.Parallel()

	// Note: imports of standard and LUCI protos are tested by more high-level
	// lucicfg tests that actually generate configs. Misc protos are not involved
	// in them, so we test they can be imported separately here.
	//
	// TODO(vadimsh): Get rid of this.
	loader := starlarkproto.NewLoader()
	if err := loader.AddDescriptorSet(miscTypesDescSet); err != nil {
		panic(err)
	}
	for _, path := range miscProtos {
		Convey(fmt.Sprintf("%q is importable", path), t, func(c C) {
			_, err := loader.Module(path)
			So(err, ShouldBeNil)
		})
	}

	// Note: testMessage() is used by other tests. This test verifies it works
	// at all.
	Convey("testMessage works", t, func() {
		i, err := testMessage(123).Attr("i")
		So(err, ShouldBeNil)
		asInt, err := starlark.AsInt32(i)
		So(err, ShouldBeNil)
		So(asInt, ShouldEqual, 123)
	})

	Convey("Doc URL works", t, func() {
		name, doc := protoMessageDoc(testMessage(123))
		So(name, ShouldEqual, "Msg")
		So(doc, ShouldEqual, "https://example.com/proto-doc") // see misc/support/test.proto
	})
}
