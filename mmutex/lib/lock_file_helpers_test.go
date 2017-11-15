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
		lockFilePath, drainFilePath, err := computeMutexPaths(env)
		So(err, ShouldBeNil)
		So(lockFilePath, ShouldBeBlank)
		So(drainFilePath, ShouldBeBlank)
	})

	Convey("computeMutexPaths returns empty strings when lock dir doesn't exist", t, func() {
		tempDir, err := ioutil.TempDir("", "")
		So(err, ShouldBeNil)
		So(os.Remove(tempDir), ShouldBeNil)

		env := subcommands.Env{
			"MMUTEX_LOCK_DIR": subcommands.EnvVar{tempDir, true},
		}
		lockFilePath, drainFilePath, err := computeMutexPaths(env)
		So(lockFilePath, ShouldBeBlank)
		So(drainFilePath, ShouldBeBlank)
		So(err, ShouldBeNil)
	})

	Convey("computeMutexPaths returns env variable based path", t, func() {
		tempDir, err := ioutil.TempDir("", "")
		defer os.Remove(tempDir)
		So(err, ShouldBeNil)

		env := subcommands.Env{
			"MMUTEX_LOCK_DIR": subcommands.EnvVar{tempDir, true},
		}
		lockFilePath, drainFilePath, err := computeMutexPaths(env)
		So(err, ShouldBeNil)
		So(lockFilePath, ShouldEqual, filepath.Join(tempDir, "mmutex.lock"))
		So(drainFilePath, ShouldEqual, filepath.Join(tempDir, "mmutex.drain"))
	})

	Convey("computeMutexPaths returns error when lock dir is a relative path", t, func() {
		path := filepath.Join("a", "b", "c")
		env := subcommands.Env{
			"MMUTEX_LOCK_DIR": subcommands.EnvVar{path, true},
		}
		lockFilePath, drainFilePath, err := computeMutexPaths(env)
		So(err, ShouldErrLike, fmt.Sprintf("Lock file directory %s must be an absolute path", path))
		So(lockFilePath, ShouldBeBlank)
		So(drainFilePath, ShouldBeBlank)
	})
}
