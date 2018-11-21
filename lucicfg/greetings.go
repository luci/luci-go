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
	"go.starlark.net/starlark"
)

// This is just a demo of how to use declNative. To be deleted when there's some
// real code.

func init() {
	declNative("emit_greeting", func(call nativeCall) (starlark.Value, error) {
		var msg starlark.String
		if err := call.unpack(1, &msg); err != nil {
			return nil, err
		}
		call.State.Greetings = append(call.State.Greetings, msg.GoString())
		return starlark.None, nil
	})
}
