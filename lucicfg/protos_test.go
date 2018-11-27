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

	"go.chromium.org/luci/starlark/interpreter"

	. "github.com/smartystreets/goconvey/convey"
)

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
}
