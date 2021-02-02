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
	"os"
	"path/filepath"
	"testing"

	"github.com/maruel/subcommands"
	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/luci/client/cmd/swarming/lib"
	"go.chromium.org/luci/hardcoded/chromeinfra"
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
	args = append([]string{cmd, "-server", "chromium-swarm-dev.appspot.com"}, args...)
	return subcommands.Run(getApplication(chromeinfra.DefaultAuthOptions()), args)
}

func TestBotsCommand(t *testing.T) {
	t.Parallel()
	Convey(`ok`, t, func() {
		dir := t.TempDir()
		jsonPath := filepath.Join(dir, "out.json")

		So(runCmd(t, "bots", "-quiet", "-json", jsonPath), ShouldEqual, 0)
	})
}

func TestTasksCommand(t *testing.T) {
	t.Parallel()
	Convey(`ok`, t, func() {
		dir := t.TempDir()
		jsonPath := filepath.Join(dir, "out.json")

		So(runCmd(t, "tasks", "-limit", "1", "-quiet", "-json", jsonPath), ShouldEqual, 0)
	})
}

func TestTriggerCommand(t *testing.T) {
	// Unset SWARMING_TASK_ID otherwise the trigger may fail for parent task association.
	parentTaskID := os.Getenv(lib.TaskIDEnvVar)
	os.Unsetenv(lib.TaskIDEnvVar)
	t.Cleanup(func() {
		os.Setenv(lib.TaskIDEnvVar, parentTaskID)
	})
	Convey(`ok`, t, func() {
		args := []string{
			"-d", "pool=chromium.tests",
			"-d", "os=Linux",
			"-idempotent",
			"--", "/bin/echo", "hi",
		}
		So(runCmd(t, "trigger", args...), ShouldEqual, 0)
	})
}
