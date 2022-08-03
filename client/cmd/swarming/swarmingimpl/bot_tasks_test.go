// Copyright 2022 The LUCI Authors.
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

package swarmingimpl

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func botTasksExpectErr(argv []string, errLike string) {
	b := botTasksRun{}
	b.Init(&testAuthFlags{})
	fullArgv := append([]string{"-server", "http://localhost:9050"}, argv...)
	err := b.GetFlags().Parse(fullArgv)
	So(err, ShouldBeNil)
	So(b.Parse(), ShouldErrLike, errLike)
}

func TestBotTasksParse(t *testing.T) {
	Convey(`Make sure that Parse fails with invalid -id.`, t, func() {
		botTasksExpectErr([]string{"-id", ""}, "non-empty -id")
	})

	Convey(`Make sure that Parse fails with -quiet without -json.`, t, func() {
		botTasksExpectErr([]string{"-id", "device_id", "-quiet"}, "specify -json")
	})

	Convey(`Make sure that Parse fails with negative -limit.`, t, func() {
		botTasksExpectErr([]string{"-id", "device_id", "-limit", "-1"}, "invalid -limit")
	})
}
