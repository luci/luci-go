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
	"os"
	"path/filepath"
	"testing"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestMain(t *testing.T) {
	ftt.Run("computeMutexPaths returns empty strings without environment variable set", t, func(t *ftt.Test) {
		env := subcommands.Env{
			"MMUTEX_LOCK_DIR": subcommands.EnvVar{"", false},
		}
		lockFilePath, drainFilePath, err := computeMutexPaths(env)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, lockFilePath, should.BeZero)
		assert.Loosely(t, drainFilePath, should.BeZero)
	})

	ftt.Run("computeMutexPaths returns empty strings when lock dir doesn't exist", t, func(t *ftt.Test) {
		tempDir, err := os.MkdirTemp("", "")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, os.Remove(tempDir), should.BeNil)

		env := subcommands.Env{
			"MMUTEX_LOCK_DIR": subcommands.EnvVar{tempDir, true},
		}
		lockFilePath, drainFilePath, err := computeMutexPaths(env)
		assert.Loosely(t, lockFilePath, should.BeZero)
		assert.Loosely(t, drainFilePath, should.BeZero)
		assert.Loosely(t, err, should.BeNil)
	})

	ftt.Run("computeMutexPaths returns env variable based path", t, func(t *ftt.Test) {
		tempDir, err := os.MkdirTemp("", "")
		defer os.Remove(tempDir)
		assert.Loosely(t, err, should.BeNil)

		env := subcommands.Env{
			"MMUTEX_LOCK_DIR": subcommands.EnvVar{tempDir, true},
		}
		lockFilePath, drainFilePath, err := computeMutexPaths(env)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, lockFilePath, should.Equal(filepath.Join(tempDir, "mmutex.lock")))
		assert.Loosely(t, drainFilePath, should.Equal(filepath.Join(tempDir, "mmutex.drain")))
	})

	ftt.Run("computeMutexPaths returns error when lock dir is a relative path", t, func(t *ftt.Test) {
		path := filepath.Join("a", "b", "c")
		env := subcommands.Env{
			"MMUTEX_LOCK_DIR": subcommands.EnvVar{path, true},
		}
		lockFilePath, drainFilePath, err := computeMutexPaths(env)
		assert.Loosely(t, err, should.ErrLike(fmt.Sprintf("Lock file directory %s must be an absolute path", path)))
		assert.Loosely(t, lockFilePath, should.BeZero)
		assert.Loosely(t, drainFilePath, should.BeZero)
	})
}
