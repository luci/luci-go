// Copyright 2017 The LUCI Authors.
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
	"context"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/danjacques/gofslock/fslock"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestShared(t *testing.T) {
	var fnThatReturns = func(err error) func(context.Context) error {
		return func(context.Context) error {
			return err
		}
	}

	ftt.Run("RunShared", t, func(t *ftt.Test) {
		lockFileDir, err := ioutil.TempDir("", "")
		assert.Loosely(t, err, should.BeNil)
		defer os.Remove(lockFileDir)
		env := subcommands.Env{
			LockFileEnvVariable: subcommands.EnvVar{
				Value:  lockFileDir,
				Exists: true,
			},
		}
		lockFilePath, drainFilePath, err := computeMutexPaths(env)
		assert.Loosely(t, err, should.BeNil)
		ctx := context.Background()

		t.Run("returns error from the command", func(t *ftt.Test) {
			assert.Loosely(t, RunShared(ctx, env, fnThatReturns(errors.Reason("test error").Err())), should.ErrLike("test error"))
		})

		t.Run("times out if an exclusive lock isn't released", func(t *ftt.Test) {
			handle, err := fslock.Lock(lockFilePath)
			assert.Loosely(t, err, should.BeNil)
			defer handle.Unlock()

			ctx, cancel := context.WithTimeout(ctx, time.Millisecond)
			defer cancel()
			assert.Loosely(t, RunShared(ctx, env, fnThatReturns(nil)), should.ErrLike("fslock: lock is held"))
			assert.Loosely(t, ctx.Err(), should.ErrLike(context.DeadlineExceeded))
		})

		t.Run("uses context parameter as basis for new context", func(t *ftt.Test) {
			ctx, cancel := context.WithCancel(ctx)
			cancel()
			err := RunShared(ctx, env, func(ctx context.Context) error {
				return clock.Sleep(ctx, time.Millisecond).Err
			})
			assert.Loosely(t, err, should.ErrLike(context.Canceled))
		})

		t.Run("executes the command if shared lock already held", func(t *ftt.Test) {
			handle, err := fslock.LockShared(lockFilePath)
			assert.Loosely(t, err, should.BeNil)
			defer handle.Unlock()

			assert.Loosely(t, RunShared(ctx, env, fnThatReturns(nil)), should.BeNil)
		})

		t.Run("waits for drain file to go away before requesting lock", func(t *ftt.Test) {
			file, err := os.OpenFile(drainFilePath, os.O_RDONLY|os.O_CREATE, 0666)
			assert.Loosely(t, err, should.BeNil)
			err = file.Close()
			assert.Loosely(t, err, should.BeNil)

			commandResult := make(chan error)
			runSharedErr := make(chan error)
			go func() {
				runSharedErr <- RunShared(ctx, env, func(ctx context.Context) error {
					// Block RunShared() immediately after it starts executing the command.
					return <-commandResult
				})
			}()

			// Sleep a millisecond so that RunShared() has an opportunity to reach the
			// logic where it checks for a drain file.
			clock.Sleep(ctx, time.Millisecond)

			// The lock should be available: RunShared() didn't acquire it due to the presence
			// of the drain file. Verify this by acquiring and immediately releasing the lock.
			handle, err := fslock.Lock(lockFilePath)
			assert.Loosely(t, err, should.BeNil)
			err = handle.Unlock()
			assert.Loosely(t, err, should.BeNil)

			// Removing the drain file should allow RunShared() to progress as normal.
			err = os.Remove(drainFilePath)
			assert.Loosely(t, err, should.BeNil)

			commandResult <- nil
			assert.Loosely(t, <-runSharedErr, should.BeNil)
		})

		t.Run("times out if drain file doesn't go away", func(t *ftt.Test) {
			file, err := os.OpenFile(drainFilePath, os.O_RDONLY|os.O_CREATE, 0666)
			assert.Loosely(t, err, should.BeNil)
			err = file.Close()
			assert.Loosely(t, err, should.BeNil)
			defer os.Remove(drainFilePath)

			ctx, cancel := context.WithTimeout(ctx, 5*time.Millisecond)
			defer cancel()
			runSharedErr := make(chan error)
			go func() {
				runSharedErr <- RunShared(ctx, env, fnThatReturns(nil))
			}()

			assert.Loosely(t, <-runSharedErr, should.ErrLike("timed out waiting for drain file to disappear"))
		})

		t.Run("acts as a passthrough if lockFileDir is empty", func(t *ftt.Test) {
			assert.Loosely(t, RunShared(ctx, subcommands.Env{}, fnThatReturns(nil)), should.BeNil)
		})
	})
}
