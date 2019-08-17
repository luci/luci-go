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
	"context"
	"fmt"
	"testing"

	"go.starlark.net/starlark"

	"go.chromium.org/luci/starlark/interpreter"
	"go.chromium.org/luci/starlark/starlarkproto"

	_ "go.chromium.org/luci/lucicfg/testproto"

	. "github.com/smartystreets/goconvey/convey"
)

// Register protos used exclusively from tests.
func init() {
	publicProtos["test.proto"] = struct {
		protoPkg string
		goPath   string
		docURL   string
	}{
		"testproto",
		"go.chromium.org/luci/lucicfg/testproto/test.proto",
		"https://example.com/proto-doc",
	}
}

// testMessage returns new testproto.Msg as a Starlark value to be used from
// tests.
func testMessage(i int) *starlarkproto.Message {
	testproto, _, err := protoLoader()("test.proto")
	if err != nil {
		panic(err)
	}
	msgT, err := testproto["testproto"].(starlark.HasAttrs).Attr("Msg")
	if err != nil {
		panic(err)
	}
	msg := msgT.(*starlarkproto.MessageType).Message()
	if err := msg.SetField("i", starlark.MakeInt(i)); err != nil {
		panic(err)
	}
	return msg
}

func TestProtosAreImportable(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	for path, proto := range publicProtos {
		Convey(fmt.Sprintf("%q (aka %q) is importable", path, proto.goPath), t, func(c C) {
			_, err := Generate(ctx, Inputs{
				Code: interpreter.MemoryLoader(map[string]string{
					"proto.star": fmt.Sprintf(`load("@proto//%s", "%s")`, path, proto.protoPkg),
				}),
				Entry: "proto.star",
			})
			// If this assertion failed, you either:
			//   1. Moved some *.proto files. This is fine, just update publicProtos
			//      mapping in protos.go.
			//   2. Renamed proto package to something else. This is not fine
			//      currently. It is a breaking change to the Starlark API exposed by
			//      the config generator.
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
}
