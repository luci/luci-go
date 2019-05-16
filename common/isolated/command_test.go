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

package isolated

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestReplaceParameters(t *testing.T) {
	t.Parallel()

	Convey("replaceParameters", t, func() {
		ctx := context.Background()

		Convey("test EXECUTABLE_SUFFIX", func() {
			arg, err := replaceParameters(ctx, "program${EXECUTABLE_SUFFIX}", "", "")
			So(err, ShouldBeNil)
			if runtime.GOOS == "windows" {
				So(arg, ShouldEqual, "program.exe")
			} else {
				So(arg, ShouldEqual, "program")
			}
		})

		Convey("test ISOLATED_OUTDIR", func() {
			arg, err := replaceParameters(ctx, "${ISOLATED_OUTDIR}/result.txt", "out", "")
			So(err, ShouldBeNil)
			So(arg, ShouldEqual, filepath.Join("out", "result.txt"))
		})

		Convey("test SWARMING_BOT_FILE", func() {
			arg, err := replaceParameters(ctx, "${SWARMING_BOT_FILE}/config", "", "cfgdir")
			So(err, ShouldBeNil)
			So(arg, ShouldEqual, filepath.Join("cfgdir", "config"))
		})
	})
}

func TestProcessCommand(t *testing.T) {
	t.Parallel()

	Convey("processCommand", t, func() {
		ctx := context.Background()
		args, err := processCommand(ctx, []string{
			"program${EXECUTABLE_SUFFIX}",
			"${ISOLATED_OUTDIR}/result.txt",
			"${SWARMING_BOT_FILE}/config",
		}, "out", "cfgdir")

		So(err, ShouldBeNil)

		executableSuffix := ""
		if runtime.GOOS == "windows" {
			executableSuffix = ".exe"
		}

		So(args, ShouldResemble, []string{
			"program" + executableSuffix,
			filepath.Join("out", "result.txt"),
			filepath.Join("cfgdir", "config"),
		})
	})
}
