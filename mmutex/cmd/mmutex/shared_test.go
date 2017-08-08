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
	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
)

func TestShared(t *testing.T) {
	Convey("RunShared executes the command", t, func() {
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

		So(RunShared(command, 0, 0), ShouldBeNil)

		_, err = os.Stat(testFilePath)
		So(err, ShouldBeNil)
	})

	Convey("RunShared returns error from the command", t, func() {
		So(RunShared([]string{"nonexistent_command"}, 0, 0), ShouldErrLike, "executable file not found")
	})

	Convey("RunShared times out if an exclusive lock isn't released", t, func() {
		var handle fslock.Handle
		var err error

		if handle, err = fslock.Lock(LockFilePath); err != nil {
			panic(err)
		}
		defer handle.Unlock()

		start := time.Now()
		So(RunShared([]string{"echo", "should_fail"}, 5*time.Millisecond, 0), ShouldErrLike, "fslock: lock is held")
		So(time.Now(), ShouldHappenOnOrAfter, start.Add(5*time.Millisecond))
	})

	Convey("RunShared executes the command if shared lock already held", t, func() {
		var handle fslock.Handle
		var err error

		if handle, err = fslock.LockShared(LockFilePath); err != nil {
			panic(err)
		}
		defer handle.Unlock()

		So(RunShared(createCommand([]string{"echo", "should_succeed"}), 0, 0), ShouldBeNil)
	})

	// TODO(charliea): Add a test to ensure that RunShared() treats the presence of a drain file the
	// same as a held lock.

	// TODO(charliea): Add a test to ensure that RunShared() uses the $CHOPS_SERVICE_LOCK environment
	// variable to determine the directory for the lock and drain files.

	// TODO(charliea): Add a test to ensure that RunShared() just acts as a passthrough if
	// the $CHOPS_SERVICE_LOCK directory isn't set.

	// TODO(charliea): Add a test to ensure that RunShared() just acts as a passthrough if
	// the $CHOPS_SERVICE_LOCK directory doesn't exist.
}
