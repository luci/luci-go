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

package interpreter

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMutable(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		_, logs, err := runIntr(intrParams{
			stdlib: map[string]string{
				"init.star": `
					load("builtin:imported.star", "append", "collect")
					append(1)
					append(2)
					print(collect())
				`,
				"imported.star": `
					_state = mutable([])
					def append(i):
						_state.get().append(i)
					def collect():
						return _state.get()
				`,
			},
		})
		So(err, ShouldBeNil)
		So(logs, ShouldResemble, []string{"[builtin:init.star:5] [1, 2]"})
	})

	Convey("Setter and boolean cast work", t, func() {
		So(runScript(`
			def test():
				s = mutable()
				if s:
					fail("should be false-ish")

				s.set(123)
				if s.get() != 123:
					fail("should be 123")
				if not s:
					fail("should be true-ish")

				s.set(0)
				if s.get() != 0:
					fail("should be 0")
				if not s:
					fail("should be true-ish")  # holds a value, even if it's 0!

				s.set(None)
				if s.get() != None:
					fail("should be None")
				if s:
					fail("should be false-ish")  # doesn't hold a value anymore

			test()  # starlark doesn't allow 'if' in global scope, need a function
		`), ShouldBeNil)
	})
}
