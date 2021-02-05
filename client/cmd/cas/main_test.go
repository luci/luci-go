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
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/maruel/subcommands"
	. "github.com/smartystreets/goconvey/convey"
)

// IntegrationTestEnvVar is the name of the environment variable which controls
// whether integaration tests are executed.
// The value must be "1" for integration tests to run.
const IntegrationTestEnvVar = "INTEGRATION_TESTS"

// runIntegrationTests true if integration tests should run.
func runIntegrationTests() bool {
	return os.Getenv(IntegrationTestEnvVar) == "1"
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

func archiveFile(t *testing.T) string {
	uploadDir := t.TempDir()
	ioutil.WriteFile(filepath.Join(uploadDir, "foo"), []byte("hi"), 0600)

	outDir := t.TempDir()
	digestPath := filepath.Join(outDir, "out.digest")
	statsJSONPath := filepath.Join(outDir, "stats.json")

	flags := []string{
		"-dump-digest",
		digestPath,
		"-dump-stats-json",
		statsJSONPath,
		"-paths",
		fmt.Sprintf("%s:%s", uploadDir, "foo"),
	}
	So(runCmd(t, "archive", flags...), ShouldEqual, 0)

	digest, err := ioutil.ReadFile(digestPath)
	So(err, ShouldBeNil)
	return string(digest)
}

func TestCASCommand(t *testing.T) {
	t.Parallel()

	Convey(`archive`, t, func() {
		digest := archiveFile(t)
		So(digest, ShouldNotBeEmpty)
	})

	Convey(`download`, t, func() {
		dir := t.TempDir()
		statsJSONPath := filepath.Join(dir, "stats.json")
		outdir := filepath.Join(dir, "out")

		digest := archiveFile(t)

		flags := []string{
			"-digest",
			digest,
			"-dump-stats-json",
			statsJSONPath,
			"-dir",
			outdir,
		}
		So(runCmd(t, "download", flags...), ShouldEqual, 0)
	})
}
