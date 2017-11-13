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

	"github.com/maruel/subcommands"
	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/luci/mmutex/lib"
)

func TestExclusive(t *testing.T) {
	Convey("RunExclusive", t, func() {
		var lockFileDir string
		var err error
		if lockFileDir, err = ioutil.TempDir("", ""); err != nil {
			panic(err)
		}
		env := subcommands.Env{
			lib.LockFileEnvVariable: subcommands.EnvVar{
				Value:  lockFileDir,
				Exists: true,
			},
		}
		defer os.Remove(lockFileDir)

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

			So(RunExclusive(env, command), ShouldBeNil)

			_, err = os.Stat(testFilePath)
			So(err, ShouldBeNil)
		})
	})
}
