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

	"go.chromium.org/luci/common/errors"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestShared(t *testing.T) {
	var fnThatReturns = func(err error) func() error {
		return func() error {
			return err
		}
	}

	Convey("RunShared", t, func() {
		var lockFileDir string
		var err error
		if lockFileDir, err = ioutil.TempDir("", ""); err != nil {
			panic(err)
		}
		env := subcommands.Env{
			LockFileEnvVariable: subcommands.EnvVar{
				Value:  lockFileDir,
				Exists: true,
			},
		}
		lockFilePath, _, err := computeMutexPaths(env)
		if err != nil {
			panic(err)
		}
		defer func() {
			os.Remove(lockFileDir)
		}()

		Convey("returns error from the command", func() {
			So(RunShared(env, fnThatReturns(errors.Reason("test error").Err())), ShouldErrLike, "test error")
		})

		Convey("times out if an exclusive lock isn't released", func() {
			var handle fslock.Handle
			var err error

			if handle, err = fslock.Lock(lockFilePath); err != nil {
				panic(err)
			}
			defer handle.Unlock()

			oldFslockTimeout := fslockTimeout
			fslockTimeout = 5 * time.Millisecond
			defer func() {
				fslockTimeout = oldFslockTimeout
			}()

			start := time.Now()
			So(RunShared(env, fnThatReturns(nil)), ShouldErrLike, "fslock: lock is held")
			So(time.Now(), ShouldHappenOnOrAfter, start.Add(5*time.Millisecond))
		})

		Convey("executes the command if shared lock already held", func() {
			var handle fslock.Handle
			var err error

			if handle, err = fslock.LockShared(lockFilePath); err != nil {
				panic(err)
			}
			defer handle.Unlock()

			So(RunShared(env, fnThatReturns(nil)), ShouldBeNil)
		})

		// TODO(charliea): Add a test to ensure that RunShared() treats the presence of a drain file the
		// same as a held lock.
	})

	Convey("RunExclusive acts as a passthrough if lockFileDir is empty", t, func() {
		So(RunShared(subcommands.Env{}, fnThatReturns(nil)), ShouldBeNil)
	})
}
