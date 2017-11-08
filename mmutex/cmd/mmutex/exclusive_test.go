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
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/danjacques/gofslock/fslock"
	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
)

func TestExclusive(t *testing.T) {
	Convey("RunExclusive", t, func() {
		var lockFileDir string
		var err error
		if lockFileDir, err = ioutil.TempDir("", ""); err != nil {
			panic(err)
		}
		lockFilePath := filepath.Join(lockFileDir, "mmutex.lock")
		drainFilePath := filepath.Join(lockFileDir, "mmutex.drain")

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

			So(RunExclusive(command, lockFilePath, drainFilePath, 0, 0), ShouldBeNil)

			_, err = os.Stat(testFilePath)
			So(err, ShouldBeNil)
		})

		Convey("returns error from the command", func() {
			So(RunExclusive([]string{"nonexistent_command"}, lockFilePath, drainFilePath, 0, 0), ShouldErrLike, "executable file not found")
		})

		Convey("times out if exclusive lock isn't released", func() {
			var handle fslock.Handle
			var err error

			if handle, err = fslock.Lock(lockFilePath); err != nil {
				panic(err)
			}
			defer handle.Unlock()

			So(RunExclusive([]string{"echo", "should_fail"}, lockFilePath, drainFilePath, 0, 0), ShouldErrLike, "fslock: lock is held")
		})

		Convey("times out if shared lock isn't released", func() {
			var handle fslock.Handle
			var err error

			if handle, err = fslock.LockShared(lockFilePath); err != nil {
				panic(err)
			}
			defer handle.Unlock()

			So(RunExclusive(createCommand([]string{"echo", "should_fail"}), lockFilePath, drainFilePath, 0, 0), ShouldErrLike, "fslock: lock is held")
		})

		Convey("respects timeout", func() {
			var handle fslock.Handle
			var err error

			if handle, err = fslock.Lock(lockFilePath); err != nil {
				panic(err)
			}
			defer handle.Unlock()

			start := time.Now()
			RunExclusive(createCommand([]string{"echo", "should_succeed"}), lockFilePath, drainFilePath, 5*time.Millisecond, 0)
			So(time.Now(), ShouldHappenOnOrAfter, start.Add(5*time.Millisecond))
		})

		Convey("creates drainfile", func() {
			// Start goroutine that calls RunExclusive(), waiting for a file to exist.
			done := make(chan bool)

			awaitedFilePath := filepath.Join(lockFileDir, "awaitedFilePath.txt")
			fmt.Println(awaitedFilePath)
			go func(done chan bool) {
				RunExclusive(createCommand([]string{"./await_file.sh", awaitedFilePath}), lockFilePath, drainFilePath, 5*time.Second, 0)
				done <- true
			}(done)

			_, err := os.Stat(drainFilePath)
			So(err, ShouldBeNil)

			os.OpenFile(awaitedFilePath, os.O_CREATE, 0755)
			<-done
		})

		Convey("deletes drainfile", func() {
			// Start goroutine that calls RunExclusive(), waiting for a file to exist.
			// Create the file.
			// Check that the drain file doesn't exist.
			// Check that the command was successful.
		})

		Convey("waits for no drainfile", func() {
			// Start goroutine that calls RunExclusive(), waiting for a file to exist.
			// Call RunExclusive() on this thread with a low timeout. Verify that it times out.
			// Create the file.
		})
	})
}
