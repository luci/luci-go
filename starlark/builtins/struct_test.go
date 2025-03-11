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
	"go.starlark.net/syntax"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

const successTest = `
def assert(a, b):
	if a != b:
		fail("%s (%s) != %s (%s)" % (a, type(a), b, type(b)))

mystruct = genstruct("mystruct")
s = mystruct(a=1, b=2)
assert(ctor(s), mystruct)
assert(ctor(1), None)
assert(ctor(struct(a=1, b=2)), "struct")
`

func TestStruct(t *testing.T) {
	t.Parallel()

	runScript := func(code string) error {
		_, err := starlark.ExecFileOptions(&syntax.FileOptions{}, &starlark.Thread{}, "main", code,
			starlark.StringDict{
				"struct":    Struct,
				"genstruct": GenStruct,
				"ctor":      Ctor,

				"fail": Fail, // for 'assert'
			})
		return err
	}

	ftt.Run("Success", t, func(t *ftt.Test) {
		assert.Loosely(t, runScript(successTest), should.BeNil)
	})
}
