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

package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/danjacques/gofslock/fslock"
	"github.com/maruel/subcommands"
	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
)

func TestShared(t *testing.T) {
	Convey("RunShared", t, func() {
		var lockFileDir string
		var err error
		if lockFileDir, err = ioutil.TempDir("", ""); err != nil {
			panic(err)
		}
		env := subcommands.Env{
			lockFileEnvVariable: subcommands.EnvVar{
				Value:  lockFileDir,
				Exists: true,
			},
		}
		lockFilePath, err := computeLockFilePath(env)
		if err != nil {
			panic(err)
		}
		defer func() {
			os.Remove(lockFileDir)
		}()

		Convey("executes the command", func() {
			var tempDir string
			var err error

			if tempDir, err = ioutil.TempDir("", ""); err != nil {
				panic(err)
			}
			defer os.Remove(tempDir)

			testFilePath := filepath.Join(tempDir, "test")
			var command []string
			if runtime.GOOS == "windows" {
				command = createCommand([]string{"copy", "NUL", testFilePath})
			} else {
				command = createCommand([]string{"touch", testFilePath})
			}

			So(RunShared(command, env, 0, 0), ShouldBeNil)

			_, err = os.Stat(testFilePath)
			So(err, ShouldBeNil)
		})

		Convey("returns error from the command", func() {
			So(RunShared([]string{"nonexistent_command"}, env, 0, 0), ShouldErrLike, "executable file not found")
		})

		Convey("times out if an exclusive lock isn't released", func() {
			var handle fslock.Handle
			var err error

			if handle, err = fslock.Lock(lockFilePath); err != nil {
				panic(err)
			}
			defer handle.Unlock()

			start := time.Now()
			So(RunShared([]string{"echo", "should_fail"}, env, 5*time.Millisecond, 0), ShouldErrLike, "fslock: lock is held")
			So(time.Now(), ShouldHappenOnOrAfter, start.Add(5*time.Millisecond))
		})

		Convey("executes the command if shared lock already held", func() {
			var handle fslock.Handle
			var err error

			if handle, err = fslock.LockShared(lockFilePath); err != nil {
				panic(err)
			}
			defer handle.Unlock()

			So(RunShared(createCommand([]string{"echo", "should_succeed"}), env, 0, 0), ShouldBeNil)
		})

		// TODO(charliea): Add a test to ensure that RunShared() treats the presence of a drain file the
		// same as a held lock.
	})

	Convey("RunExclusive acts as a passthrough if lockFilePath is empty", t, func() {
		So(RunShared(createCommand([]string{"echo", "should_succeed"}), subcommands.Env{}, 0, 0), ShouldBeNil)
	})
}
