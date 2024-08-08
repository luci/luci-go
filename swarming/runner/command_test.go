// Copyright 2021 The LUCI Authors.
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

package runner

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestReplaceCommandParameters(t *testing.T) {
	t.Parallel()

	ftt.Run("ReplaceCommandParameters", t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run("test EXECUTABLE_SUFFIX", func(t *ftt.Test) {
			arg, err := ReplaceCommandParameters(ctx, "program${EXECUTABLE_SUFFIX}", "", "")
			assert.Loosely(t, err, should.BeNil)
			if runtime.GOOS == "windows" {
				assert.Loosely(t, arg, should.Equal("program.exe"))
			} else {
				assert.Loosely(t, arg, should.Equal("program"))
			}
		})

		t.Run("test ISOLATED_OUTDIR", func(t *ftt.Test) {
			arg, err := ReplaceCommandParameters(ctx, "${ISOLATED_OUTDIR}/result.txt", "out", "")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, arg, should.Equal(filepath.Join("out", "result.txt")))
		})

		t.Run("test SWARMING_BOT_FILE", func(t *ftt.Test) {
			arg, err := ReplaceCommandParameters(ctx, "${SWARMING_BOT_FILE}/config", "", "cfgdir")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, arg, should.Equal(filepath.Join("cfgdir", "config")))
		})
	})
}

func TestProcessCommand(t *testing.T) {
	t.Parallel()

	ftt.Run("ProcessCommand", t, func(t *ftt.Test) {
		ctx := context.Background()
		args, err := ProcessCommand(ctx, []string{
			"program${EXECUTABLE_SUFFIX}",
			"${ISOLATED_OUTDIR}/result.txt",
			"${SWARMING_BOT_FILE}/config",
		}, "out", "cfgdir")

		assert.Loosely(t, err, should.BeNil)

		executableSuffix := ""
		if runtime.GOOS == "windows" {
			executableSuffix = ".exe"
		}

		assert.Loosely(t, args, should.Resemble([]string{
			"program" + executableSuffix,
			filepath.Join("out", "result.txt"),
			filepath.Join("cfgdir", "config"),
		}))
	})
}
