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

func TestTasksParse(t *testing.T) {
	Convey(`Make sure that Parse fails with zero -limit.`, t, func() {
		t := tasksRun{}
		t.Init(&testAuthFlags{})
		err := t.GetFlags().Parse([]string{"-server", "http://localhost:9050", "-limit", "0"})
		So(err, ShouldBeNil)
		So(t.Parse(), ShouldErrLike, "must be positive")
	})

	Convey(`Make sure that Parse fails with negative -limit.`, t, func() {
		t := tasksRun{}
		t.Init(&testAuthFlags{})
		err := t.GetFlags().Parse([]string{"-server", "http://localhost:9050", "-limit", "-1"})
		So(err, ShouldBeNil)
		So(t.Parse(), ShouldErrLike, "must be positive")
	})

	Convey(`Make sure that Parse fails with -quiet without -json.`, t, func() {
		t := tasksRun{}
		t.Init(&testAuthFlags{})
		err := t.GetFlags().Parse([]string{"-server", "http://localhost:9050", "-quiet"})
		So(err, ShouldBeNil)
		So(t.Parse(), ShouldErrLike, "specify -json")
	})
}
