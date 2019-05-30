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
)

func TestFail(t *testing.T) {
	t.Parallel()

	runScript := func(code string) error {
		th := &starlark.Thread{}
		fc := &FailureCollector{}
		fc.Install(th)
		_, err := starlark.ExecFile(th, "main", code, starlark.StringDict{
			"fail":       Fail,
			"stacktrace": Stacktrace,
		})
		if f := fc.LatestFailure(); f != nil {
			return f
		}
		return err
	}

	Convey("Works with default trace", t, func() {
		err := runScript(`fail("boo")`)
		So(err.Error(), ShouldEqual, "boo")
		SkipSo(err.(*Failure).Backtrace(), ShouldContainSubstring, "main:1:5: in <toplevel>")
	})

	Convey("Works with custom trace", t, func() {
		err := runScript(`
def capture():
	return stacktrace()

s = capture()

fail("boo", trace=s)
`)
		So(err.Error(), ShouldEqual, "boo")
		SkipSo(err.(*Failure).Backtrace(), ShouldContainSubstring, "main:3:19: in capture")
	})
}
