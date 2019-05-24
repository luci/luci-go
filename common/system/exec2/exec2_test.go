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

package exec2

import (
	"context"
	"runtime"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestExec(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip()
	}

	t.Parallel()

	Convey("TestExec", t, func() {
		ctx := context.Background()
		Convey("normal", func() {
			cmd := CommandContext(ctx, "echo")
			So(cmd.Start(), ShouldBeNil)

			So(cmd.Wait(time.Second), ShouldBeNil)

			So(cmd.ProcessState.ExitCode(), ShouldEqual, 0)
		})

		Convey("timeout", func() {
			cmd := CommandContext(ctx, "sleep", "1000")

			So(cmd.Start(), ShouldBeNil)

			So(cmd.Wait(time.Millisecond), ShouldEqual, ErrTimeout)

			So(cmd.Terminate(), ShouldBeNil)

			So(cmd.Wait(time.Second).Error(), ShouldEqual, "signal: terminated")

			So(cmd.ProcessState.ExitCode(), ShouldEqual, -1)
		})

		Convey("context timeout", func() {
			ctx, cancel := context.WithTimeout(ctx, time.Millisecond)
			defer cancel()
			cmd := CommandContext(ctx, "sleep", "1000")

			So(cmd.Start(), ShouldBeNil)

			So(cmd.Wait(time.Second).Error(), ShouldEqual, "signal: killed")

			So(cmd.ProcessState.ExitCode(), ShouldEqual, -1)
		})

	})
}
