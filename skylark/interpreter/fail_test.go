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

	"github.com/google/skylark"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFail(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		err := runScript(`fail("boo")`)
		So(err.(*skylark.EvalError).Msg, ShouldEqual, "boo")
	})

	Convey("Does type cast", t, func() {
		err := runScript(`fail(123)`)
		So(err.(*skylark.EvalError).Msg, ShouldEqual, "123")
	})

	Convey("Wants 1 arg", t, func() {
		err := runScript(`fail(123, 456)`)
		So(err.(*skylark.EvalError).Msg, ShouldEqual, "'fail' got 2 arguments, wants 1")
	})

	Convey("Doesn't like kwargs", t, func() {
		err := runScript(`fail(msg=123)`)
		So(err.(*skylark.EvalError).Msg, ShouldEqual, "'fail' doesn't accept kwargs")
	})
}
