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

// Package main is a client to a Swarming server.
//
// The reference server python implementation documentation can be found at
// https://github.com/luci/luci-py/tree/master/appengine/swarming/doc
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/maruel/subcommands"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/testfs"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

// integrationTestEnvVar is the name of the environment variable which controls
// whether integaration tests are executed.
// The value must be "1" for integration tests to run.
const integrationTestEnvVar = "INTEGRATION_TESTS"

// runIntegrationTests true if integration tests should run.
func runIntegrationTests() bool {
	return os.Getenv(integrationTestEnvVar) == "1"
}

// runCmd runs swarming commands appending common flags.
// It skips if integration should not run.
func runCmd(cmd string, args ...string) int {
	args = append([]string{cmd, "-cas-instance", "chromium-swarm-dev"}, args...)
	return subcommands.Run(getApplication(), args)
}

func TestArchiveAndDownloadCommand(t *testing.T) {
	if !runIntegrationTests() {
		t.Skipf("Skip integration tests")
	}
	t.Parallel()

	ftt.Run("archive", t, func(t *ftt.Test) {
		uploadDir := t.TempDir()
		os.WriteFile(filepath.Join(uploadDir, "foo"), []byte("hi"), 0600)

		outDir := t.TempDir()
		digestPath := filepath.Join(outDir, "out.digest")
		uploadJSONPath := filepath.Join(outDir, "upload-stats.json")

		archiveFlags := []string{
			"-dump-digest",
			digestPath,
			"-dump-json",
			uploadJSONPath,
			"-paths",
			fmt.Sprintf("%s:%s", uploadDir, "foo"),
		}
		assert.Loosely(t, runCmd("archive", archiveFlags...), should.BeZero)

		digestBytes, err := os.ReadFile(digestPath)
		assert.Loosely(t, err, should.BeNil)
		digest := string(digestBytes)
		assert.Loosely(t, digest, should.Equal("1ec750cf7c4ebc220a753a705229574873b3c650b939cf4a69df1af84b40b981/77"))

		t.Run("and download", func(t *ftt.Test) {
			downloadJSONPath := filepath.Join(outDir, "download-stats.json")
			downloadDir := filepath.Join(outDir, "out")

			downloadFlags := []string{
				"-digest",
				digest,
				"-dump-json",
				downloadJSONPath,
				"-dir",
				downloadDir,
			}
			assert.Loosely(t, runCmd("download", downloadFlags...), should.BeZero)
			layout, err := testfs.Collect(downloadDir)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, layout, should.Resemble(map[string]string{"foo": "hi"}))
		})
	})
}
