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

func TestExclusive(t *testing.T) {
	var fnThatReturns = func(err error) func(context.Context) error {
		return func(context.Context) error {
			return err
		}
	}

	Convey("RunExclusive", t, func() {
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
			So(RunExclusive(ctx, env, fnThatReturns(errors.Reason("test error").Err())), ShouldErrLike, "test error")
		})

		Convey("times out if exclusive lock isn't released", func() {
			handle, err := fslock.Lock(lockFilePath)
			So(err, ShouldBeNil)
			defer handle.Unlock()

			ctx, _ = context.WithTimeout(ctx, time.Millisecond)
			So(RunExclusive(ctx, env, fnThatReturns(nil)), ShouldErrLike, "fslock: lock is held")
		})

		Convey("times out if shared lock isn't released", func() {
			handle, err := fslock.LockShared(lockFilePath)
			So(err, ShouldBeNil)
			defer handle.Unlock()

			ctx, _ = context.WithTimeout(ctx, time.Millisecond)
			So(RunExclusive(ctx, env, fnThatReturns(nil)), ShouldErrLike, "fslock: lock is held")
		})

		Convey("uses context parameter as basis for new context", func() {
			ctx, cancel := context.WithCancel(ctx)
			cancel()
			err := RunExclusive(ctx, env, func(ctx context.Context) error {
				return clock.Sleep(ctx, time.Millisecond).Err
			})
			So(err, ShouldErrLike, "context canceled")
		})

		Convey("respects timeout", func() {
			handle, err := fslock.Lock(lockFilePath)
			So(err, ShouldBeNil)
			defer handle.Unlock()

			ctx, _ = context.WithTimeout(ctx, 5*time.Millisecond)
			start := time.Now()
			RunExclusive(ctx, env, fnThatReturns(nil))
			So(time.Now(), ShouldHappenOnOrAfter, start.Add(5*time.Millisecond))
		})

		Convey("creates drain file while acquiring the lock", func() {
			handle, err := fslock.LockShared(lockFilePath)
			So(err, ShouldBeNil)
			defer handle.Unlock()

			go func() {
				RunExclusive(ctx, env, fnThatReturns(nil))
			}()

			// Sleep for a millisecond to allow time for the drain file to be created.
			clock.Sleep(ctx, 3*time.Millisecond)

			_, err = os.Stat(drainFilePath)
			So(err, ShouldBeNil)
		})

		Convey("removes drain file immediately after acquiring the lock", func() {
			commandResult := make(chan error)
			runExclusiveErr := make(chan error)
			go func() {
				runExclusiveErr <- RunExclusive(ctx, env, func(ctx context.Context) error {
					// Block RunExclusive() immediately after it starts executing the command.
					// The drain file should have been cleaned up immediately before this point.
					return <-commandResult
				})
			}()

			// Sleep to allow time for RunExclusive to acquire the lock and block on our
			// channel.
			clock.Sleep(ctx, 3*time.Millisecond)

			_, err = os.Stat(drainFilePath)
			So(os.IsNotExist(err), ShouldBeTrue)

			commandResult <- nil
			So(<-runExclusiveErr, ShouldBeNil)
		})

		Convey("acts as a passthrough if lockFileDir is empty", func() {
			So(RunExclusive(ctx, subcommands.Env{}, fnThatReturns(nil)), ShouldBeNil)
		})
	})
}
