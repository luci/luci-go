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

package lib

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func tasksExpectErr(argv []string, errLike string) {
	t := tasksRun{}
	t.Init(&testAuthFlags{})
	fullArgv := append([]string{"-server", "http://localhost:9050"}, argv...)
	err := t.GetFlags().Parse(fullArgv)
	So(err, ShouldBeNil)
	So(t.Parse(), ShouldErrLike, errLike)
}

func TestTasksParse(t *testing.T) {
	Convey(`Make sure that Parse fails with zero -limit.`, t, func() {
		tasksExpectErr([]string{"-limit", "0"}, "must be positive")
	})

	Convey(`Make sure that Parse fails with negative -limit.`, t, func() {
		tasksExpectErr([]string{"-limit", "-1"}, "must be positive")
	})

	Convey(`Make sure that Parse fails with -quiet without -json.`, t, func() {
		tasksExpectErr([]string{"-quiet"}, "specify -json")
	})

	Convey(`Make sure that Parse requires positive -start with -count.`, t, func() {
		tasksExpectErr([]string{"-count"}, "provide -start")
		tasksExpectErr([]string{"-count", "-start", "0"}, "provide -start")
		tasksExpectErr([]string{"-count", "-start", "-1"}, "provide -start")
	})

	Convey(`Make sure that Parse forbids -field or -limit to be used with -count.`, t, func() {
		tasksExpectErr([]string{"-count", "-start", "1337", "-field", "items/task_id"}, "-field cannot")
		tasksExpectErr([]string{"-count", "-start", "1337", "-limit", "100"}, "-limit cannot")
	})
}
