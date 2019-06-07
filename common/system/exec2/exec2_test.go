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
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func build(src string) (string, error) {
	tmpdir, err := ioutil.TempDir("", "test")
	if err != nil {
		return "", err
	}

	binary := filepath.Join(tmpdir, "exe.exe")
	cmd := exec.Command("go", "build", "-o", binary, src)
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return binary, nil
}

func TestExec(t *testing.T) {
	t.Parallel()

	Convey("TestExec", t, func() {
		ctx := context.Background()

		var err error
		var testBinary string

		defer func() {
			So(os.RemoveAll(filepath.Dir(testBinary)), ShouldBeNil)
		}()

		Convey("exit", func() {
			testBinary, err = build(filepath.Join("testdata", "exit.go"))
			So(err, ShouldBeNil)

			Convey("exit 0", func() {
				cmd := CommandContext(ctx, testBinary)
				So(cmd.Start(), ShouldBeNil)

				So(cmd.Wait(time.Second), ShouldBeNil)

				So(cmd.ExitCode(), ShouldEqual, 0)

			})

			Convey("exit 42", func() {
				cmd := CommandContext(ctx, testBinary, "42")
				So(cmd.Start(), ShouldBeNil)

				So(cmd.Wait(time.Second), ShouldBeError, "exit status 42")

				So(cmd.ExitCode(), ShouldEqual, 42)
			})
		})

		Convey("timeout", func() {
			testBinary, err = build(filepath.Join("testdata", "timeout.go"))
			So(err, ShouldBeNil)

			cmd := CommandContext(ctx, testBinary)

			So(cmd.Start(), ShouldBeNil)

			So(cmd.Wait(time.Millisecond), ShouldEqual, ErrTimeout)

			So(cmd.Terminate(), ShouldBeNil)

			if runtime.GOOS == "windows" {
				So(cmd.Wait(time.Second), ShouldBeError, "exit status 1")
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
			testBinary, err = build(filepath.Join("testdata", "timeout.go"))
			So(err, ShouldBeNil)

			if runtime.GOOS == "windows" {
				// TODO(tikuta): support context timeout on windows
				return
			}

			ctx, cancel := context.WithTimeout(ctx, time.Millisecond)
			defer cancel()

			cmd := CommandContext(ctx, testBinary)

			So(cmd.Start(), ShouldBeNil)

			So(cmd.Wait(time.Second).Error(), ShouldEqual, "signal: killed")

			So(cmd.ExitCode(), ShouldEqual, -1)
		})

	})
}
