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
		var logs []string
		err := execInitScripts(map[string]string{
			"init.sky": `
load("builtin:imported.sky", "append", "collect")
append(1)
append(2)
print(collect())
`,
			"imported.sky": `
_state = mutable([])
def append(i):
  _state.get().append(i)
def collect():
  return _state.get()
`,
		}, &logs)
		So(err, ShouldBeNil)
		So(logs, ShouldResemble, []string{"[1, 2]"})
	})

	Convey("Setter and boolean cast work", t, func() {
		So(execInitScript(`
def test():
  s = mutable()
  if s:
    fail("should be falsish")
  s.set(123)
  if s.get() != 123:
    fail("should be 123")
  if not s:
    fail("should be trueish")

test()  # skylark doesn't allow 'if' in global scope, need a function
`), ShouldBeNil)
	})
}
