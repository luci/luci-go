// Copyright 2023 The LUCI Authors.
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

package exec_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"runtime"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/common/exec"
	"go.chromium.org/luci/common/exec/execmock"
	"go.chromium.org/luci/common/system/environ"
	. "go.chromium.org/luci/common/testing/assertions"
)

var panicRunner = execmock.Register(func(_ execmock.None) (_ execmock.None, exitcode int, err error) {
	panic("big boom")
})

func TestCmd(t *testing.T) {
	args := []string{"echo", "hello", "there"}
	if runtime.GOOS == "windows" {
		args = append([]string{"cmd.exe", "/c"}, args...)
	}
	fullArgs := args
	var prog string
	prog, args = args[0], args[1:]

	t.Parallel()

	Convey(`Cmd`, t, func() {
		ctx := execmock.Init(context.Background())

		// b/351223612: Broken on Windows.
		SkipConvey(`works with pasthrough`, func() {
			execmock.Passthrough.Mock(ctx)

			cmd := exec.Command(ctx, prog, args...)
			data, err := cmd.Output()
			So(err, ShouldBeNil)
			So(bytes.TrimSpace(data), ShouldResemble, []byte("hello there"))

			cmd = exec.Command(ctx, prog, args...)
			data, err = cmd.CombinedOutput()
			So(err, ShouldBeNil)
			So(bytes.TrimSpace(data), ShouldResemble, []byte("hello there"))
		})
		Convey(`works with mocking`, func() {
			execmock.Simple.
				WithArgs("echo").
				WithLimit(1).
				Mock(ctx, execmock.SimpleInput{
					Stdout: "not what you expected",
				})

			cmd := exec.Command(ctx, prog, args...)
			data, err := cmd.CombinedOutput()
			So(err, ShouldBeNil)
			So(string(data), ShouldResemble, "not what you expected")

			Convey(`and still can't run it twice`, func() {
				So(cmd.Start(), ShouldErrLike, "exec: already started")
			})
		})

		Convey(`mocking without a fallback generates an error`, func() {
			cmd := exec.CommandContext(ctx, prog, args...)
			So(cmd.Run(), ShouldErrLike, "no mock matches")
		})

		Convey(`can collect the remaining mocks`, func() {
			echoSingle := execmock.Simple.WithArgs("echo").WithLimit(1)

			notExpected := echoSingle.Mock(ctx, execmock.SimpleInput{
				Stdout: "not what you expected",
			})

			different := echoSingle.Mock(ctx, execmock.SimpleInput{
				Stdout: "a different outcome",
			})

			exit100 := echoSingle.Mock(ctx, execmock.SimpleInput{
				ExitCode: 100,
			})

			cmd := exec.CommandContext(ctx, prog, args...)
			data, err := cmd.Output()
			So(err, ShouldBeNil)
			So(string(data), ShouldResemble, "not what you expected")

			So(notExpected.Snapshot(), ShouldHaveLength, 1)
			So(different.Snapshot(), ShouldHaveLength, 0)
			So(exit100.Snapshot(), ShouldHaveLength, 0)

			cmd = exec.CommandContext(ctx, prog, args...)
			data, err = cmd.Output()
			So(err, ShouldBeNil)
			So(string(data), ShouldResemble, "a different outcome")

			So(notExpected.Snapshot(), ShouldHaveLength, 1)
			So(different.Snapshot(), ShouldHaveLength, 1)
			So(exit100.Snapshot(), ShouldHaveLength, 0)

			cmd = exec.CommandContext(ctx, prog, args...)
			_, err = cmd.Output()
			So(err, ShouldErrLike, "exit status 100")
			So(cmd.ProcessState.ExitCode(), ShouldEqual, 100)

			So(notExpected.Snapshot(), ShouldHaveLength, 1)
			So(different.Snapshot(), ShouldHaveLength, 1)
			So(exit100.Snapshot(), ShouldHaveLength, 1)
		})

		Convey(`can detect misses`, func() {
			cmd := exec.CommandContext(ctx, prog, args...)
			So(cmd.Run(), ShouldErrLike, "no mock matches")

			misses := execmock.ResetState(ctx)
			So(misses, ShouldHaveLength, 1)
			So(misses[0].Args, ShouldResemble, fullArgs)
		})

		Convey(`can simulate a startup error`, func() {
			execmock.StartError.WithArgs("echo").WithLimit(1).Mock(ctx, errors.New("start boom"))

			cmd := exec.CommandContext(ctx, prog, args...)
			So(cmd.Run(), ShouldErrLike, "start boom")

			Convey(`which persists`, func() {
				So(cmd.Start(), ShouldErrLike, "start boom")
			})
		})

		Convey(`can use StdoutPipe`, func() {
			execmock.Simple.Mock(ctx, execmock.SimpleInput{
				Stdout: "hello world",
			})

			cmd := exec.CommandContext(ctx, prog, args...)
			out, err := cmd.StdoutPipe()
			So(err, ShouldBeNil)

			So(cmd.Start(), ShouldBeNil)
			data, err := io.ReadAll(out)
			So(cmd.Wait(), ShouldBeNil)
			So(err, ShouldBeNil)
			So(string(data), ShouldResemble, "hello world")
		})

		Convey(`can use StderrPipe`, func() {
			execmock.Simple.Mock(ctx, execmock.SimpleInput{
				Stderr: "hello world",
			})

			cmd := exec.CommandContext(ctx, prog, args...)
			out, err := cmd.StderrPipe()
			So(err, ShouldBeNil)

			So(cmd.Start(), ShouldBeNil)
			data, err := io.ReadAll(out)
			So(cmd.Wait(), ShouldBeNil)
			So(err, ShouldBeNil)
			So(string(data), ShouldResemble, "hello world")
		})

		Convey(`can see panic`, func() {
			uses := panicRunner.Mock(ctx)

			cmd := exec.CommandContext(ctx, prog, args...)
			out, err := cmd.StdoutPipe()
			So(err, ShouldBeNil)

			So(cmd.Start(), ShouldBeNil)
			data, err := io.ReadAll(out)
			So(err, ShouldBeNil)
			So(data, ShouldBeEmpty)

			So(cmd.Wait(), ShouldErrLike, "exit status 1")

			_, _, panicStack := uses.Snapshot()[0].GetOutput(context.Background())
			So(panicStack, ShouldNotBeEmpty)
		})
	})

}

func TestMain(m *testing.M) {
	if environ.System().Get("EXEC_TEST_SELF_CALL") == "1" {
		os.Stdout.WriteString("EXEC_TEST_SELF_CALL")
		os.Exit(0)
	}
	execmock.Intercept()
	os.Exit(m.Run())
}
