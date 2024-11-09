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
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/mmutex/lib"
)

func TestShared(t *testing.T) {
	ftt.Run("RunShared", t, func(t *ftt.Test) {
		lockFileDir, err := ioutil.TempDir("", "")
		assert.Loosely(t, err, should.BeNil)
		defer os.Remove(lockFileDir)
		env := subcommands.Env{
			lib.LockFileEnvVariable: subcommands.EnvVar{
				Value:  lockFileDir,
				Exists: true,
			},
		}

		t.Run("executes the command", func(t *ftt.Test) {
			tempDir, err := ioutil.TempDir("", "")
			assert.Loosely(t, err, should.BeNil)
			defer os.Remove(tempDir)

			testFilePath := filepath.Join(tempDir, "test")
			var command []string
			if runtime.GOOS == "windows" {
				command = createCommand([]string{"copy", "NUL", testFilePath})
			} else {
				command = createCommand([]string{"touch", testFilePath})
			}

			assert.Loosely(t, RunShared(context.Background(), env, command), should.BeNil)

			_, err = os.Stat(testFilePath)
			assert.Loosely(t, err, should.BeNil)
		})
	})
}
