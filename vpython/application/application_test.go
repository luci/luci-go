// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package application

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.chromium.org/luci/cipd/client/cipd"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/filesystem"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/vpython/api/vpython"
)

func TestParseArguments(t *testing.T) {
	ftt.Run("Test parse arguments", t, func(t *ftt.Test) {
		ctx := context.Background()

		app := &Application{}
		app.Initialize(ctx)

		parseArgs := func(args ...string) error {
			app.Arguments = args
			assert.Loosely(t, app.ParseEnvs(ctx), should.BeNil)
			return app.ParseArgs(ctx)
		}

		t.Run("Test log level", func(t *ftt.Test) {
			err := parseArgs(
				"-vpython-log-level",
				"warning",
			)
			assert.Loosely(t, err, should.BeNil)
			ctx = app.SetLogLevel(ctx)
			assert.Loosely(t, logging.GetLevel(ctx), should.Equal(logging.Warning))
		})

		t.Run("Test unknown argument", func(t *ftt.Test) {
			const unknownErr = "failed to extract flags: unknown flag: vpython-test"

			// Care but only care arguments begin with "-" or "--".
			err := parseArgs("-vpython-test")
			assert.Loosely(t, err, should.ErrLike(unknownErr))
			err = parseArgs("--vpython-test")
			assert.Loosely(t, err, should.ErrLike(unknownErr))
			err = parseArgs("-vpython-root", "root", "vpython-test")
			assert.Loosely(t, err, should.BeNil)

			// All arguments after the script file should be bypassed.
			err = parseArgs("-vpython-test", "test.py")
			assert.Loosely(t, err, should.ErrLike(unknownErr))
			err = parseArgs("test.py", "-vpython-test")
			assert.Loosely(t, err, should.BeNil)

			// Stop parsing arguments when seen --
			err = parseArgs("--", "-vpython-test")
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("Test default cache dir", func(t *ftt.Test) {
			app.userCacheDir = func() (string, error) { return "<CACHE>", nil }
			app.getuid = func() int { return 10 }

			err := app.ParseEnvs(ctx)
			assert.Loosely(t, err, should.BeNil)
			err = app.ParseArgs(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, app.VpythonRoot, should.HaveSuffix(filepath.Join("<CACHE>", "vpython-root.10")))
		})

		t.Run("Test no user cache dir - with uid", func(t *ftt.Test) {
			app.userCacheDir = func() (string, error) { return "", errors.New("error") }
			app.getuid = func() int { return 10 }

			err := app.ParseEnvs(ctx)
			assert.Loosely(t, err, should.BeNil)
			err = app.ParseArgs(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, app.VpythonRoot, should.HaveSuffix("vpython-root_10"))
		})

		t.Run("Test no user cache dir - without uid", func(t *ftt.Test) {
			app.userCacheDir = func() (string, error) { return "", errors.New("error") }
			app.getuid = func() int { return -1 }

			err := app.ParseEnvs(ctx)
			assert.Loosely(t, err, should.BeNil)
			err = app.ParseArgs(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, app.VpythonRoot, should.NotBeEmpty)
		})

		t.Run("Test cipd cache dir", func(t *ftt.Test) {
			err := parseArgs("-vpython-root", "root", "vpython-test")
			assert.Loosely(t, err, should.BeNil)
			wd, err := os.Getwd()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, app.CIPDCacheDir, should.HavePrefix(filepath.Join(wd, "root")))
		})

		t.Run("Test cipd cache dir with env", func(t *ftt.Test) {
			// Don't set cipd cache dir if env provides one
			app.Environments = append(app.Environments, fmt.Sprintf("%s=%s", cipd.EnvCacheDir, "something"))
			err := parseArgs("-vpython-root", "root", "vpython-test")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, app.CIPDCacheDir, should.HavePrefix("something"))
		})

		t.Run("Test spec load", func(t *ftt.Test) {
			t.Run("not found", func(t *ftt.Test) {
				wd, err := os.Getwd()
				assert.Loosely(t, err, should.BeNil)
				defer os.Chdir(wd)
				err = os.Chdir(t.TempDir())
				assert.Loosely(t, err, should.BeNil)

				// CommonFilesystemBarrier for spec loader
				err = filesystem.Touch(".gclient", time.Time{}, fs.ModePerm)
				assert.Loosely(t, err, should.BeNil)

				err = parseArgs()
				assert.Loosely(t, err, should.BeNil)

				t.Run("default", func(t *ftt.Test) {
					app.VpythonSpec = &vpython.Spec{PythonVersion: "something"}
					err = app.LoadSpec(ctx)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, app.VpythonSpec.GetPythonVersion(), should.Equal("something"))
				})

				t.Run("no default", func(t *ftt.Test) {
					err = app.LoadSpec(ctx)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, app.VpythonSpec, should.NotBeNil)
				})
			})
		})
	})
}
