// Copyright 2019 The LUCI Authors.
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
	"go.starlark.net/starlark"

	"go.chromium.org/luci/starlark/interpreter"
)

func init() {
	// See //internal/io.star.
	declNative("read_file", func(call nativeCall) (starlark.Value, error) {
		var path starlark.String
		if err := call.unpack(1, &path); err != nil {
			return nil, err
		}
		intr := interpreter.GetThreadInterpreter(call.Thread)
		src, err := intr.LoadSource(call.Thread, path.GoString())
		return starlark.String(src), err
	})
}
