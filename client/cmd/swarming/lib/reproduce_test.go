// Copyright 2021 The LUCI Authors.
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
	"os"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func init() {
	// So that this test works on swarming!
	os.Unsetenv(ServerEnvVar)
	os.Unsetenv(TaskIDEnvVar)
}

func TestReproduceParse_NoArgs(t *testing.T) {
	t.Parallel()
	Convey(`Make sure Parse works with no arguments.`, t, func() {
		c := reproduceRun{}
		c.Init(&testAuthFlags{})

		err := c.parse([]string(nil))
		So(err, ShouldErrLike, "must provide -server")
	})
}

func TestReproduceParse_NoTaskID(t *testing.T) {
	t.Parallel()
	Convey(`Make sure Parse works with with no task ID.`, t, func() {
		c := reproduceRun{}
		c.Init(&testAuthFlags{})

		err := c.GetFlags().Parse([]string{"-server", "http://localhost:9050"})

		err = c.parse([]string(nil))
		So(err, ShouldErrLike, "must specify exactly one task id.")
	})
}
