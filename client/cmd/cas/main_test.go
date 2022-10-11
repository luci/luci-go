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
	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/common/testing/testfs"
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
func runCmd(t *testing.T, cmd string, args ...string) int {
	if !runIntegrationTests() {
		t.Skipf("Skip integration tests")
	}
	args = append([]string{cmd, "-cas-instance", "chromium-swarm-dev"}, args...)
	return subcommands.Run(getApplication(), args)
}

func TestArchiveAndDownloadCommand(t *testing.T) {
	t.Parallel()

	Convey("archive", t, func() {
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
		So(runCmd(t, "archive", archiveFlags...), ShouldEqual, 0)

		digestBytes, err := os.ReadFile(digestPath)
		So(err, ShouldBeNil)
		digest := string(digestBytes)
		So(digest, ShouldEqual, "1ec750cf7c4ebc220a753a705229574873b3c650b939cf4a69df1af84b40b981/77")

		Convey("and download", func() {
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
			So(runCmd(t, "download", downloadFlags...), ShouldEqual, 0)
			layout, err := testfs.Collect(downloadDir)
			So(err, ShouldBeNil)
			So(layout, ShouldResemble, map[string]string{"foo": "hi"})
		})
	})
}
