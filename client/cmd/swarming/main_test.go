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
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/maruel/subcommands"
	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/luci/client/cmd/swarming/lib"
	"go.chromium.org/luci/common/api/swarming/swarming/v1"
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
	args = append([]string{cmd, "-server", "chromium-swarm-dev.appspot.com", "-quiet"}, args...)
	return subcommands.Run(getApplication(chromeinfra.DefaultAuthOptions()), args)
}

func TestBotsCommand(t *testing.T) {
	t.Parallel()
	Convey(`ok`, t, func() {
		dir := t.TempDir()
		jsonPath := filepath.Join(dir, "out.json")

		So(runCmd(t, "bots", "-json", jsonPath), ShouldEqual, 0)
	})
}

func TestTasksCommand(t *testing.T) {
	t.Parallel()
	Convey(`ok`, t, func() {
		dir := t.TempDir()
		jsonPath := filepath.Join(dir, "out.json")

		So(runCmd(t, "tasks", "-limit", "1", "-json", jsonPath), ShouldEqual, 0)
	})
}

// triggerTask triggers a task and returns the triggered TaskRequest.
func triggerTask(t *testing.T) *swarming.SwarmingRpcsTaskRequestMetadata {
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "out.json")
	args := []string{
		"-d", "pool=chromium.tests",
		"-d", "os=Linux",
		"-dump-json", jsonPath,
		"-idempotent",
		"--", "/bin/echo", "hi",
	}
	So(runCmd(t, "trigger", args...), ShouldEqual, 0)

	results := readTriggerResults(jsonPath)
	So(results.Tasks, ShouldHaveLength, 1)
	return results.Tasks[0]
}

// readTriggerResults reads TriggerResults from output json file.
func readTriggerResults(jsonPath string) *lib.TriggerResults {
	resultsJSON, err := ioutil.ReadFile(jsonPath)
	So(err, ShouldBeNil)

	results := &lib.TriggerResults{}
	err = json.Unmarshal(resultsJSON, results)
	So(err, ShouldBeNil)

	return results
}

// unsetParentTaskID unset SWARMING_TASK_ID environment variable, otherwise task trigger may fail for parent task association.
func unsetParentTaskID(t *testing.T) {
	parentTaskID := os.Getenv(lib.TaskIDEnvVar)
	os.Unsetenv(lib.TaskIDEnvVar)
	t.Cleanup(func() {
		os.Setenv(lib.TaskIDEnvVar, parentTaskID)
	})
}

func TestTriggerCommand(t *testing.T) {
	unsetParentTaskID(t)
	Convey(`ok`, t, func() {
		triggerTask(t)
	})
}

func TestCollectCommand(t *testing.T) {
	unsetParentTaskID(t)
	Convey(`ok`, t, func() {
		triggeredTask := triggerTask(t)
		dir := t.TempDir()
		So(runCmd(t, "collect", "-output-dir", dir, triggeredTask.TaskId), ShouldEqual, 0)
	})
}

func TestRequestShowCommand(t *testing.T) {
	unsetParentTaskID(t)
	Convey(`ok`, t, func() {
		triggeredTask := triggerTask(t)
		So(runCmd(t, "request_show", triggeredTask.TaskId), ShouldEqual, 0)
	})
}

const spawnTaskInputJSON = `
{
  "requests": [
    {
      "name": "spawn-task test",
      "priority": "200",
      "task_slices": [
        {
          "expiration_secs": "21600",
          "properties": {
            "dimensions": [
              {"key": "pool", "value": "chromium.tests"},
              {"key": "os", "value": "Linux"}
            ],
            "command": ["/bin/echo", "hi"],
            "execution_timeout_secs": "3600",
            "idempotent": true
          }
        }
      ]
    },
    {
      "name": "spawn-task test",
      "priority": "200",
      "task_slices": [
        {
          "expiration_secs": "21600",
          "properties": {
            "dimensions": [
              {"key": "pool", "value": "chromium.tests"},
              {"key": "os", "value": "Linux"}
            ],
            "command": ["/bin/echo", "hi"],
            "execution_timeout_secs": "3600",
            "idempotent": true
          }
        }
      ]
    }
  ]
}
`

func TestSpawnTasksCommand(t *testing.T) {
	unsetParentTaskID(t)
	Convey(`ok`, t, func() {
		// prepare input file.
		dir := t.TempDir()
		inputPath := filepath.Join(dir, "input.json")
		err := ioutil.WriteFile(inputPath, []byte(spawnTaskInputJSON), 0600)
		So(err, ShouldBeNil)

		outputPath := filepath.Join(dir, "output.json")
		So(runCmd(t, "spawn_task", "-json-input", inputPath, "-json-output", outputPath), ShouldEqual, 0)

		results := readTriggerResults(outputPath)
		So(results.Tasks, ShouldHaveLength, 2)
	})
}
