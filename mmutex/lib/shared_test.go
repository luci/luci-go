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
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/danjacques/gofslock/fslock"
	"github.com/maruel/subcommands"
	"golang.org/x/net/context"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestShared(t *testing.T) {
	var fnThatReturns = func(err error) func(context.Context) error {
		return func(context.Context) error {
			return err
		}
	}

	Convey("RunShared", t, func() {
		lockFileDir, err := ioutil.TempDir("", "")
		So(err, ShouldBeNil)
		defer os.Remove(lockFileDir)
		env := subcommands.Env{
			LockFileEnvVariable: subcommands.EnvVar{
				Value:  lockFileDir,
				Exists: true,
			},
		}
		lockFilePath, drainFilePath, err := computeMutexPaths(env)
		So(err, ShouldBeNil)
		ctx := context.Background()

		Convey("returns error from the command", func() {
			So(RunShared(ctx, env, fnThatReturns(errors.Reason("test error").Err())), ShouldErrLike, "test error")
		})

		Convey("times out if an exclusive lock isn't released", func() {
			handle, err := fslock.Lock(lockFilePath)
			So(err, ShouldBeNil)
			defer handle.Unlock()

			ctx, _ = context.WithTimeout(ctx, 5*time.Millisecond)
			start := time.Now()
			So(RunShared(ctx, env, fnThatReturns(nil)), ShouldErrLike, "fslock: lock is held")
			So(time.Now(), ShouldHappenOnOrAfter, start.Add(5*time.Millisecond))
		})

		Convey("uses context parameter as basis for new context", func() {
			ctx, cancel := context.WithCancel(ctx)
			cancel()
			err := RunShared(ctx, env, func(ctx context.Context) error {
				return clock.Sleep(ctx, time.Millisecond).Err
			})
			So(err, ShouldErrLike, "context canceled")
		})

		Convey("executes the command if shared lock already held", func() {
			handle, err := fslock.LockShared(lockFilePath)
			So(err, ShouldBeNil)
			defer handle.Unlock()

			So(RunShared(ctx, env, fnThatReturns(nil)), ShouldBeNil)
		})

		Convey("waits for drain file to go away before requesting lock", func() {
			file, err := os.OpenFile(drainFilePath, os.O_RDONLY|os.O_CREATE, 0666)
			So(err, ShouldBeNil)
			err = file.Close()
			So(err, ShouldBeNil)

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
			So(err, ShouldBeNil)
			err = handle.Unlock()
			So(err, ShouldBeNil)

			// Removing the drain file should allow RunShared() to progress as normal.
			err = os.Remove(drainFilePath)
			So(err, ShouldBeNil)

			commandResult <- nil
			So(<-runSharedErr, ShouldBeNil)
		})

		Convey("times out if drain file doesn't go away", func() {
			file, err := os.OpenFile(drainFilePath, os.O_RDONLY|os.O_CREATE, 0666)
			So(err, ShouldBeNil)
			err = file.Close()
			So(err, ShouldBeNil)
			defer os.Remove(drainFilePath)

			ctx, _ = context.WithTimeout(ctx, 5*time.Millisecond)
			runSharedErr := make(chan error)
			go func() {
				runSharedErr <- RunShared(ctx, env, fnThatReturns(nil))
			}()

			So(<-runSharedErr, ShouldErrLike, "timed out waiting for drain file to disappear")
		})

		Convey("acts as a passthrough if lockFileDir is empty", func() {
			So(RunShared(ctx, subcommands.Env{}, fnThatReturns(nil)), ShouldBeNil)
		})
	})
}
