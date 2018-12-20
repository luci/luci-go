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

package builtins

import (
	"testing"

	"go.starlark.net/starlark"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

const regexpTest = `
def assert(a, b):
  if a != b:
    fail("%s != %s" % (a, b))

def test():
  groups = submatches('a (\d+) (\d+)', 'a 123 456 tail')
  assert(groups, ("a 123 456", "123", "456"))

  groups = submatches('a (\d+) (\d+)', 'a huh 456')
  assert(groups, ())

  groups = submatches('.*', 'a 123 456 tail')
  assert(groups, ("a 123 456 tail",))

  groups = submatches('', 'zzz')
  assert(groups, ('',))  # empty match (not the same as no match at all)

test()
`

func TestRegexp(t *testing.T) {
	t.Parallel()

	submatches := RegexpMatcher("submatches")

	runScript := func(code string) error {
		_, err := starlark.ExecFile(&starlark.Thread{}, "main", code, starlark.StringDict{
			"submatches": submatches,
			"fail":       Fail, // for 'assert'
		})
		return err
	}

	Convey("Success", t, func() {
		So(runScript(regexpTest), ShouldBeNil)
	})

	Convey("Fails", t, func() {
		So(runScript(`submatches('(((', '')`), ShouldErrLike, "error parsing regexp")
	})
}
