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

func TestExclusive(t *testing.T) {
	var fnThatReturns = func(err error) func(context.Context) error {
		return func(context.Context) error {
			return err
		}
	}

	ftt.Run("RunExclusive", t, func(t *ftt.Test) {
		lockFileDir, err := os.MkdirTemp("", "")
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
			assert.Loosely(t, RunExclusive(ctx, env, fnThatReturns(errors.New("test error"))), should.ErrLike("test error"))
		})

		t.Run("times out if exclusive lock isn't released", func(t *ftt.Test) {
			handle, err := fslock.Lock(lockFilePath)
			assert.Loosely(t, err, should.BeNil)
			defer handle.Unlock()

			ctx, cancel := context.WithTimeout(ctx, time.Millisecond)
			defer cancel()
			assert.Loosely(t, RunExclusive(ctx, env, fnThatReturns(nil)), should.ErrLike("fslock: lock is held"))
		})

		t.Run("times out if shared lock isn't released", func(t *ftt.Test) {
			handle, err := fslock.LockShared(lockFilePath)
			assert.Loosely(t, err, should.BeNil)
			defer handle.Unlock()

			ctx, cancel := context.WithTimeout(ctx, time.Millisecond)
			defer cancel()
			assert.Loosely(t, RunExclusive(ctx, env, fnThatReturns(nil)), should.ErrLike("fslock: lock is held"))
		})

		t.Run("uses context parameter as basis for new context", func(t *ftt.Test) {
			ctx, cancel := context.WithCancel(ctx)
			cancel()
			err := RunExclusive(ctx, env, func(ctx context.Context) error {
				return clock.Sleep(ctx, time.Millisecond).Err
			})
			assert.Loosely(t, err, should.ErrLike(context.Canceled))
		})

		t.Run("respects timeout", func(t *ftt.Test) {
			handle, err := fslock.Lock(lockFilePath)
			assert.Loosely(t, err, should.BeNil)
			defer handle.Unlock()

			ctx, cancel := context.WithTimeout(ctx, time.Millisecond)
			defer cancel()
			RunExclusive(ctx, env, fnThatReturns(nil))
			assert.Loosely(t, ctx.Err(), should.ErrLike(context.DeadlineExceeded))
		})

		t.Run("creates drain file while acquiring the lock", func(t *ftt.Test) {
			handle, err := fslock.LockShared(lockFilePath)
			assert.Loosely(t, err, should.BeNil)
			defer handle.Unlock()

			go func() {
				RunExclusive(ctx, env, fnThatReturns(nil))
			}()

			// Sleep for a millisecond to allow time for the drain file to be created.
			clock.Sleep(ctx, 3*time.Millisecond)

			_, err = os.Stat(drainFilePath)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("removes drain file immediately after acquiring the lock", func(t *ftt.Test) {
			commandStarted := make(chan struct{})
			commandResult := make(chan error)
			runExclusiveErr := make(chan error)
			go func() {
				runExclusiveErr <- RunExclusive(ctx, env, func(ctx context.Context) error {
					close(commandStarted)

					// Block RunExclusive() immediately after it starts executing the command.
					// The drain file should have been cleaned up immediately before this point.
					return <-commandResult
				})
			}()

			// At the time when the command starts but has not yet finished executing
			// (because it's blocked on the commandResult channel), the drain file
			// should be removed.
			<-commandStarted
			_, err = os.Stat(drainFilePath)
			assert.Loosely(t, os.IsNotExist(err), should.BeTrue)

			commandResult <- nil
			assert.Loosely(t, <-runExclusiveErr, should.BeNil)
		})

		t.Run("acts as a passthrough if lockFileDir is empty", func(t *ftt.Test) {
			assert.Loosely(t, RunExclusive(ctx, subcommands.Env{}, fnThatReturns(nil)), should.BeNil)
		})
	})
}
