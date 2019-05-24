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

// TODO(tikuta): Support windows.
// +build !windows

package exec

import (
	osexec "os/exec"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestExec(t *testing.T) {
	t.Parallel()

	Convey("TestExec", t, func() {
		Convey("normal", func() {
			cmd := osexec.Command("sh")
			proc := NewProc(cmd)

			So(proc.Start(), ShouldBeNil)

			So(proc.Wait(time.Minute), ShouldBeNil)
			So(proc.ExitCode(), ShouldEqual, 0)
		})

		Convey("timeout", func() {
			cmd := osexec.Command("sleep", "1000")
			proc := NewProc(cmd)

			So(proc.Start(), ShouldBeNil)

			So(proc.Wait(time.Millisecond), ShouldEqual, ErrTimeout)

			So(proc.Terminate(), ShouldBeNil)

			So(proc.Wait(time.Millisecond).Error(), ShouldEqual, "signal: terminated")
			So(proc.ExitCode(), ShouldEqual, -1)
		})
	})
}
