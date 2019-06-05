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

package exec2

import (
	"context"
	"runtime"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestExec(t *testing.T) {
	t.Parallel()

	Convey("TestExec", t, func() {
		ctx := context.Background()

		Convey("normal", func() {
			var args []string
			if runtime.GOOS == "windows" {
				args = []string{"cmd.exe", "/c"}
			}
			args = append(args, "echo")
			cmd := CommandContext(ctx, args[0], args[1:]...)

			So(cmd.Start(), ShouldBeNil)

			So(cmd.Wait(time.Second), ShouldBeNil)

			So(cmd.ExitCode(), ShouldEqual, 0)
		})

		Convey("timeout", func() {
			var args []string
			if runtime.GOOS == "windows" {
				args = []string{"powershell.exe", "-Command", `"Start-Sleep -s 1000"`}
			} else {
				args = []string{"sleep", "1000"}
			}
			cmd := CommandContext(ctx, args[0], args[1:]...)

			So(cmd.Start(), ShouldBeNil)

			So(cmd.Wait(time.Millisecond), ShouldEqual, ErrTimeout)

			So(cmd.Terminate(), ShouldBeNil)

			if runtime.GOOS == "windows" {
				So(cmd.Wait(time.Second), ShouldBeNil)
			} else {
				So(cmd.Wait(time.Second).Error(), ShouldEqual, "signal: terminated")
			}

			if runtime.GOOS == "windows" {
				So(cmd.ExitCode(), ShouldEqual, 1)
			} else {
				So(cmd.ExitCode(), ShouldEqual, -1)
			}
		})

		Convey("context timeout", func() {
			if runtime.GOOS == "windows" {
				// TODO(tikuta): support context timeout on windows
				return
			}
			ctx, cancel := context.WithTimeout(ctx, time.Millisecond)
			defer cancel()
			cmd := CommandContext(ctx, "sleep", "1000")

			So(cmd.Start(), ShouldBeNil)

			So(cmd.Wait(time.Second).Error(), ShouldEqual, "signal: killed")

			So(cmd.ExitCode(), ShouldEqual, -1)
		})

	})
}
