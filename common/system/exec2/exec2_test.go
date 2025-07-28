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
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func build(src, tmpdir string) (string, error) {
	binary := filepath.Join(tmpdir, "exe.exe")
	cmd := exec.Command("go", "build", "-o", binary, src)
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return binary, nil
}

func TestExec(t *testing.T) {
	t.Parallel()

	ftt.Run("TestExec", t, func(t *ftt.Test) {
		ctx := context.Background()

		tmpdir := t.TempDir()

		errCh := make(chan error, 1)

		t.Run("exit", func(t *ftt.Test) {
			testBinary, err := build(filepath.Join("testdata", "exit.go"), tmpdir)
			assert.Loosely(t, err, should.BeNil)

			t.Run("exit 0", func(t *ftt.Test) {
				cmd := CommandContext(ctx, testBinary)
				assert.Loosely(t, cmd.Start(), should.BeNil)

				assert.Loosely(t, cmd.Wait(), should.BeNil)

				assert.Loosely(t, cmd.ProcessState.ExitCode(), should.BeZero)
			})

			t.Run("exit 42", func(t *ftt.Test) {
				cmd := CommandContext(ctx, testBinary, "42")
				assert.Loosely(t, cmd.Start(), should.BeNil)

				assert.Loosely(t, cmd.Wait(), should.ErrLike("exit status 42"))

				assert.Loosely(t, cmd.ProcessState.ExitCode(), should.Equal(42))
			})
		})

		t.Run("timeout", func(t *ftt.Test) {
			testBinary, err := build(filepath.Join("testdata", "timeout.go"), tmpdir)
			assert.Loosely(t, err, should.BeNil)

			cmd := CommandContext(ctx, testBinary)
			rc, err := cmd.StdoutPipe()
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, cmd.Start(), should.BeNil)

			expected := []byte("I'm alive!")
			buf := make([]byte, len(expected))
			n, err := rc.Read(buf)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, n, should.Equal(len(expected)))
			assert.Loosely(t, buf, should.Match(expected))

			assert.Loosely(t, rc.Close(), should.BeNil)

			go func() {
				errCh <- cmd.Wait()
			}()

			select {
			case err := <-errCh:
				t.Log(err)
				assert.Loosely(t, "should not reach here", should.BeNil)
			case <-time.After(time.Millisecond):
			}

			assert.Loosely(t, cmd.Terminate(), should.BeNil)

			select {
			case err := <-errCh:
				if runtime.GOOS == "windows" {
					assert.Loosely(t, err, should.ErrLike("exit status"))
				} else {
					assert.Loosely(t, err, should.ErrLike("signal: terminated"))
				}
				// The value of the exit code depends on GOOS and version of Go runtime.
				assert.Loosely(t, cmd.ProcessState.ExitCode(), should.NotEqual(0))
			case <-time.After(time.Minute):
				t.Log(err)
				assert.Loosely(t, "should not reach here", should.BeNil)
			}
		})

		t.Run("context timeout", func(t *ftt.Test) {
			testBinary, err := build(filepath.Join("testdata", "timeout.go"), tmpdir)
			assert.Loosely(t, err, should.BeNil)

			if runtime.GOOS == "windows" {
				// TODO(tikuta): support context timeout on windows
				return
			}

			ctx, cancel := context.WithTimeout(ctx, time.Millisecond)
			defer cancel()

			cmd := CommandContext(ctx, testBinary)

			assert.Loosely(t, cmd.Start(), should.BeNil)

			assert.Loosely(t, cmd.Wait(), should.ErrLike("signal: killed"))

			assert.Loosely(t, cmd.ProcessState.ExitCode(), should.Equal(-1))
		})
	})
}

func TestSetEnv(t *testing.T) {
	t.Parallel()

	ftt.Run("TestSetEnv", t, func(t *ftt.Test) {
		ctx := context.Background()

		tmpdir := t.TempDir()

		testBinary, err := build(filepath.Join("testdata", "env.go"), tmpdir)
		assert.Loosely(t, err, should.BeNil)

		cmd := CommandContext(ctx, testBinary)
		env := environ.System()
		env.Set("envvar", "envvar")
		cmd.Env = env.Sorted()

		assert.Loosely(t, cmd.Start(), should.BeNil)

		assert.Loosely(t, cmd.Wait(), should.BeNil)

		assert.Loosely(t, cmd.ProcessState.ExitCode(), should.BeZero)
	})
}
