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
	Convey("computeMutexPaths returns empty strings without environment variable set", t, func() {
		env := subcommands.Env{
			"MMUTEX_LOCK_DIR": subcommands.EnvVar{"", false},
		}
		mutexPaths, err := computeMutexPaths(env)
		So(err, ShouldBeNil)
		So(mutexPaths.lockFile, ShouldBeBlank)
		So(mutexPaths.drainFile, ShouldBeBlank)
	})

	Convey("computeMutexPaths returns empty strings when lock dir doesn't exist", t, func() {
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
		mutexPaths, err := computeMutexPaths(env)
		So(mutexPaths.lockFile, ShouldBeBlank)
		So(mutexPaths.drainFile, ShouldBeBlank)
		So(err, ShouldBeNil)
	})

	Convey("computeMutexPaths returns env variable based path", t, func() {
		var tempDir string
		var err error
		if tempDir, err = ioutil.TempDir("", ""); err != nil {
			panic(err)
		}

		env := subcommands.Env{
			"MMUTEX_LOCK_DIR": subcommands.EnvVar{tempDir, true},
		}
		mutexPaths, err := computeMutexPaths(env)
		So(err, ShouldBeNil)
		So(mutexPaths.lockFile, ShouldEqual, filepath.Join(tempDir, "mmutex.lock"))
		So(mutexPaths.drainFile, ShouldEqual, filepath.Join(tempDir, "mmutex.drain"))
	})

	Convey("computeMutexPaths returns error when lock dir is a relative path", t, func() {
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
		mutexPaths, err := computeMutexPaths(env)
		So(err, ShouldErrLike, fmt.Sprintf("Lock file directory %s must be an absolute path", relPath))
		So(mutexPaths.lockFile, ShouldBeBlank)
		So(mutexPaths.drainFile, ShouldBeBlank)
	})
}
