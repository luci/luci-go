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

	"go.chromium.org/luci/common/exec"
	"go.chromium.org/luci/common/exec/execmock"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
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

	ftt.Run(`Cmd`, t, func(t *ftt.Test) {
		ctx := execmock.Init(context.Background())

		t.Run(`works with pasthrough`, func(t *ftt.Test) {
			if runtime.GOOS == "windows" {
				t.Skip("b/351223612: Broken on Windows")
			}
			execmock.Passthrough.Mock(ctx)

			cmd := exec.Command(ctx, prog, args...)
			data, err := cmd.Output()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, bytes.TrimSpace(data), should.Match([]byte("hello there")))

			cmd = exec.Command(ctx, prog, args...)
			data, err = cmd.CombinedOutput()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, bytes.TrimSpace(data), should.Match([]byte("hello there")))
		})
		t.Run(`works with mocking`, func(t *ftt.Test) {
			execmock.Simple.
				WithArgs("echo").
				WithLimit(1).
				Mock(ctx, execmock.SimpleInput{
					Stdout: "not what you expected",
				})

			cmd := exec.Command(ctx, prog, args...)
			data, err := cmd.CombinedOutput()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, string(data), should.Match("not what you expected"))

			t.Run(`and still can't run it twice`, func(t *ftt.Test) {
				assert.Loosely(t, cmd.Start(), should.ErrLike("exec: already started"))
			})
		})

		t.Run(`mocking without a fallback generates an error`, func(t *ftt.Test) {
			cmd := exec.CommandContext(ctx, prog, args...)
			assert.Loosely(t, cmd.Run(), should.ErrLike("no mock matches"))
		})

		t.Run(`can collect the remaining mocks`, func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, string(data), should.Match("not what you expected"))

			assert.Loosely(t, notExpected.Snapshot(), should.HaveLength(1))
			assert.Loosely(t, different.Snapshot(), should.HaveLength(0))
			assert.Loosely(t, exit100.Snapshot(), should.HaveLength(0))

			cmd = exec.CommandContext(ctx, prog, args...)
			data, err = cmd.Output()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, string(data), should.Match("a different outcome"))

			assert.Loosely(t, notExpected.Snapshot(), should.HaveLength(1))
			assert.Loosely(t, different.Snapshot(), should.HaveLength(1))
			assert.Loosely(t, exit100.Snapshot(), should.HaveLength(0))

			cmd = exec.CommandContext(ctx, prog, args...)
			_, err = cmd.Output()
			assert.Loosely(t, err, should.ErrLike("exit status 100"))
			assert.Loosely(t, cmd.ProcessState.ExitCode(), should.Equal(100))

			assert.Loosely(t, notExpected.Snapshot(), should.HaveLength(1))
			assert.Loosely(t, different.Snapshot(), should.HaveLength(1))
			assert.Loosely(t, exit100.Snapshot(), should.HaveLength(1))
		})

		t.Run(`can detect misses`, func(t *ftt.Test) {
			cmd := exec.CommandContext(ctx, prog, args...)
			assert.Loosely(t, cmd.Run(), should.ErrLike("no mock matches"))

			misses := execmock.ResetState(ctx)
			assert.Loosely(t, misses, should.HaveLength(1))
			assert.Loosely(t, misses[0].Args, should.Match(fullArgs))
		})

		t.Run(`can simulate a startup error`, func(t *ftt.Test) {
			execmock.StartError.WithArgs("echo").WithLimit(1).Mock(ctx, errors.New("start boom"))

			cmd := exec.CommandContext(ctx, prog, args...)
			assert.Loosely(t, cmd.Run(), should.ErrLike("start boom"))

			t.Run(`which persists`, func(t *ftt.Test) {
				assert.Loosely(t, cmd.Start(), should.ErrLike("start boom"))
			})
		})

		t.Run(`can use StdoutPipe`, func(t *ftt.Test) {
			execmock.Simple.Mock(ctx, execmock.SimpleInput{
				Stdout: "hello world",
			})

			cmd := exec.CommandContext(ctx, prog, args...)
			out, err := cmd.StdoutPipe()
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, cmd.Start(), should.BeNil)
			data, err := io.ReadAll(out)
			assert.Loosely(t, cmd.Wait(), should.BeNil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, string(data), should.Match("hello world"))
		})

		t.Run(`can use StderrPipe`, func(t *ftt.Test) {
			execmock.Simple.Mock(ctx, execmock.SimpleInput{
				Stderr: "hello world",
			})

			cmd := exec.CommandContext(ctx, prog, args...)
			out, err := cmd.StderrPipe()
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, cmd.Start(), should.BeNil)
			data, err := io.ReadAll(out)
			assert.Loosely(t, cmd.Wait(), should.BeNil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, string(data), should.Match("hello world"))
		})

		t.Run(`can see panic`, func(t *ftt.Test) {
			uses := panicRunner.Mock(ctx)

			cmd := exec.CommandContext(ctx, prog, args...)
			out, err := cmd.StdoutPipe()
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, cmd.Start(), should.BeNil)
			data, err := io.ReadAll(out)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, data, should.BeEmpty)

			assert.Loosely(t, cmd.Wait(), should.ErrLike("exit status 1"))

			_, panicStack, _ := uses.Snapshot()[0].GetOutput(context.Background())
			assert.Loosely(t, panicStack, should.NotBeEmpty)
		})
	})

}

func TestMain(m *testing.M) {
	if environ.System().Get("EXEC_TEST_SELF_CALL") == "1" {
		os.Stdout.WriteString("EXEC_TEST_SELF_CALL")
		os.Exit(0)
	}
	execmock.Intercept(true)
	os.Exit(m.Run())
}
