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

func TestExclusive(t *testing.T) {
	Convey("RunExclusive executes the command", t, func() {
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

		So(RunExclusive(command, 0, 0), ShouldBeNil)

		_, err = os.Stat(testFilePath)
		So(err, ShouldBeNil)
	})

	Convey("RunExclusive returns error from the command", t, func() {
		So(RunExclusive([]string{"nonexistent_command"}, 0, 0), ShouldErrLike, "executable file not found")
	})

	Convey("RunExclusive times out if exclusive lock isn't released", t, func() {
		var handle fslock.Handle
		var err error

		if handle, err = fslock.Lock(LockFilePath); err != nil {
			panic(err)
		}
		defer handle.Unlock()

		So(RunExclusive([]string{"echo", "should_fail"}, 0, 0), ShouldErrLike, "fslock: lock is held")
	})

	Convey("RunExclusive times out if shared lock isn't released", t, func() {
		var handle fslock.Handle
		var err error

		if handle, err = fslock.LockShared(LockFilePath); err != nil {
			panic(err)
		}
		defer handle.Unlock()

		So(RunExclusive(createCommand([]string{"echo", "should_fail"}), 0, 0), ShouldErrLike, "fslock: lock is held")
	})

	Convey("RunExclusive respects timeout", t, func() {
		var handle fslock.Handle
		var err error

		if handle, err = fslock.Lock(LockFilePath); err != nil {
			panic(err)
		}
		defer handle.Unlock()

		start := time.Now()
		RunExclusive(createCommand([]string{"echo", "should_succeed"}), 5*time.Millisecond, 0)
		So(time.Now(), ShouldHappenOnOrAfter, start.Add(5*time.Millisecond))
	})

	// TODO(charliea): Add a test to ensure that a drain file is created when RunExclusive() is called.

	// TODO(charliea): Add a test to ensure that RunExclusive() uses the $CHOPS_SERVICE_LOCK environment
	// variable to determine the directory for the lock and drain files.

	// TODO(charliea): Add a test to ensure that RunExclusive() just acts as a passthrough if
	// the $CHOPS_SERVICE_LOCK directory isn't set.

	// TODO(charliea): Add a test to ensure that RunExclusive() just acts as a passthrough if
	// the $CHOPS_SERVICE_LOCK directory doesn't exist.
}
