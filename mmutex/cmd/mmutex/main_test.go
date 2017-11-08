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
	"testing"

	"github.com/maruel/subcommands"
	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
)

func TestMain(t *testing.T) {
	Convey("computeLockFilePathreturns empty string without environment variable set", t, func() {
		env := subcommands.Env{
			"MMUTEX_LOCK_DIR": subcommands.EnvVar{"", false},
		}
		lockFilePath, err := computeLockFilePath(env)
		So(err, ShouldBeNil)
		So(lockFilePath, ShouldBeBlank)
	})

	Convey("computeLockFilePath returns empty string when lock dir doesn't exist", t, func() {
		var tempDir string
		var err error
		if tempDir, err = ioutil.TempDir("", ""); err != nil {
			panic(err)
		}
		if err = os.Remove(tempDir); err != nil {
			panic(err)
		}

		env := subcommands.Env{
			"MMUTEX_LOCK_DIR": subcommands.EnvVar{tempDir, true},
		}
		lockFilePath, err := computeLockFilePath(env)
		So(lockFilePath, ShouldBeBlank)
		So(err, ShouldBeNil)
	})

	Convey("computeLockFilePath returns env variable based path", t, func() {
		var tempDir string
		var err error
		if tempDir, err = ioutil.TempDir("", ""); err != nil {
			panic(err)
		}

		env := subcommands.Env{
			"MMUTEX_LOCK_DIR": subcommands.EnvVar{tempDir, true},
		}
		lockFilePath, err := computeLockFilePath(env)
		So(err, ShouldBeNil)
		So(lockFilePath, ShouldEqual, filepath.Join(tempDir, "mmutex.lock"))
	})

	Convey("computeLockFilePath returns error when lock dir is a relative path", t, func() {
		var tempDir string
		var err error
		if tempDir, err = ioutil.TempDir("", ""); err != nil {
			panic(err)
		}
		var wd string
		if wd, err = os.Getwd(); err != nil {
			panic(err)
		}
		var relPath string
		if relPath, err = filepath.Rel(wd, tempDir); err != nil {
			panic(err)
		}

		env := subcommands.Env{
			"MMUTEX_LOCK_DIR": subcommands.EnvVar{relPath, true},
		}
		lockFilePath, err := computeLockFilePath(env)
		So(err, ShouldErrLike, fmt.Sprintf("Lock file directory %s must be an absolute path", relPath))
		So(lockFilePath, ShouldBeBlank)
	})
}
