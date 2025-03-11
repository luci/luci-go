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

func TestFail(t *testing.T) {
	t.Parallel()

	runScript := func(code string) error {
		th := &starlark.Thread{}
		fc := &FailureCollector{}
		fc.Install(th)
		_, err := starlark.ExecFileOptions(&syntax.FileOptions{}, th, "main", code,
			starlark.StringDict{
				"fail":       Fail,
				"stacktrace": Stacktrace,
			})
		if f := fc.LatestFailure(); f != nil {
			return f
		}
		return err
	}

	ftt.Run("Works with default trace", t, func(t *ftt.Test) {
		err := runScript(`fail("boo")`)
		assert.Loosely(t, err.Error(), should.Equal("boo"))
		assert.Loosely(t, NormalizeStacktrace(err.(*Failure).Backtrace()),
			should.ContainSubstring("main: in <toplevel>"))
	})

	ftt.Run("Works with custom trace", t, func(t *ftt.Test) {
		err := runScript(`
def capture():
	return stacktrace()

s = capture()

fail("boo", 123, ['z'], None, trace=s)
`)
		assert.Loosely(t, err.Error(), should.Equal(`boo 123 ["z"] None`))
		assert.Loosely(t, NormalizeStacktrace(err.(*Failure).Backtrace()),
			should.ContainSubstring("main: in capture"))
	})
}
