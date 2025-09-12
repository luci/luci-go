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

package main

import (
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/maruel/subcommands"
	"google.golang.org/protobuf/encoding/protojson"

	"go.chromium.org/luci/client/cmd/swarming/swarmingimpl/clipb"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/swarming/client/swarming"
	swarmingpb "go.chromium.org/luci/swarming/proto/api_v2"
)

// IntegrationTestEnvVar is the name of the environment variable which controls
// whether integaration tests are executed.
// The value must be "1" for integration tests to run.
const IntegrationTestEnvVar = "INTEGRATION_TESTS"

func init() {
	// Unset SWARMING_TASK_ID environment variable, otherwise task trigger may
	// fail for parent task association.
	err := os.Unsetenv(swarming.TaskIDEnvVar)
	if err != nil {
		log.Fatalf("Failed to unset env %v", err)
	}
}

// runIntegrationTests true if integration tests should run.
func runIntegrationTests() bool {
	return os.Getenv(IntegrationTestEnvVar) == "1"
}

// runCmd runs swarming commands appending common flags.
// It skips if integration should not run.
func runCmd(t testing.TB, cmd string, args ...string) int {
	if !runIntegrationTests() {
		t.Skipf("Skip integration tests")
	}
	args = append([]string{cmd, "-server", "chromium-swarm-dev.appspot.com", "-quiet"}, args...)
	return subcommands.Run(getApplication(), args)
}

func TestBotsCommand(t *testing.T) {
	t.Parallel()

	ftt.Run(`ok`, t, func(t *ftt.Test) {
		dir := t.TempDir()
		jsonPath := filepath.Join(dir, "out.json")

		assert.Loosely(t, runCmd(t, "bots", "-json", jsonPath), should.BeZero)
	})
}

func TestTasksCommand(t *testing.T) {
	t.Parallel()

	ftt.Run(`ok`, t, func(t *ftt.Test) {
		dir := t.TempDir()
		jsonPath := filepath.Join(dir, "out.json")

		assert.Loosely(t, runCmd(t, "tasks", "-limit", "1", "-json", jsonPath), should.BeZero)
	})
}

// triggerTask triggers a task and returns the triggered TaskRequest.
func triggerTask(t testing.TB, args []string) *swarmingpb.TaskRequestMetadataResponse {
	t.Helper()

	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "out.json")
	sbDir := t.TempDir()
	sbPath := filepath.Join(sbDir, "secret_bytes.txt")
	err := os.WriteFile(sbPath, []byte("This is secret!"), 0600)
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	args = append(args, []string{
		"-d", "pool=infra.tests",
		"-d", "os=Linux",
		"-realm", "infra:tests",
		"-dump-json", jsonPath,
		"-idempotent",
		"-secret-bytes-path", sbPath,
		"--", "/bin/bash", "-c", "echo hi > ${ISOLATED_OUTDIR}/out",
	}...)
	assert.Loosely(t, runCmd(t, "trigger", args...), should.BeZero, truth.LineContext())

	results := readTriggerResults(t, jsonPath)
	assert.Loosely(t, results.Tasks, should.HaveLength(1), truth.LineContext())
	task := results.Tasks[0]
	slice := task.Request.TaskSlices[0]
	redactedSb := []byte("<REDACTED>")
	assert.Loosely(t, slice.Properties.SecretBytes, should.Match(redactedSb), truth.LineContext())
	return task
}

// readTriggerResults reads TriggerResults from output json file.
func readTriggerResults(t testing.TB, jsonPath string) *clipb.SpawnTasksOutput {
	t.Helper()
	resultsJSON, err := os.ReadFile(jsonPath)
	assert.Loosely(t, err, should.BeNil, truth.LineContext())

	results := &clipb.SpawnTasksOutput{}
	err = protojson.Unmarshal(resultsJSON, results)
	assert.Loosely(t, err, should.BeNil, truth.LineContext())

	return results
}

func testCollectCommand(t testing.TB, taskID string) {
	t.Helper()
	dir := t.TempDir()
	assert.Loosely(t, runCmd(t, "collect", "-output-dir", dir, taskID), should.BeZero, truth.LineContext())
	out, err := os.ReadFile(filepath.Join(dir, taskID, "out"))
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	assert.Loosely(t, string(out), should.Match("hi\n"), truth.LineContext())
}

func runIntegrationTest(t testing.TB, triggerArgs []string) {
	t.Helper()
	triggeredTask := triggerTask(t, triggerArgs)
	assert.Loosely(t, runCmd(t, "request_show", triggeredTask.TaskId), should.BeZero, truth.LineContext())
	testCollectCommand(t, triggeredTask.TaskId)
}

func TestWithCAS(t *testing.T) {
	t.Parallel()
	t.Skip("no Linux bots in infra.tests dev pool")

	ftt.Run(`ok`, t, func(t *ftt.Test) {
		// TODO(jwata): ensure the digest is uploaded on CAS.
		// https://cas-viewer-dev.appspot.com/projects/chromium-swarm-dev/instances/default_instance/blobs/ad455795d66ac6d3bc0905f6a137dda1fb1d2de252a9f2a73329428fe1cf645a/77/tree
		// Use the same digest with the output so that the content is kept on
		// CAS (hopefully...)
		runIntegrationTest(t, []string{"-digest", "ad455795d66ac6d3bc0905f6a137dda1fb1d2de252a9f2a73329428fe1cf645a/77"})
	})
}

const spawnTaskInputJSON = `
{
	"requests": [
		{
			"name": "spawn-task test with CAS",
			"realm": "infra:tests",
			"priority": "200",
			"task_slices": [
				{
					"expiration_secs": "21600",
					"properties": {
						"dimensions": [
							{"key": "pool", "value": "infra.tests"},
							{"key": "os", "value": "Linux"}
						],
						"cas_input_root": {
							"cas_instance": "projects/chromium-swarm-dev/instances/default_instance",
							"digest": {
								"hash": "ad455795d66ac6d3bc0905f6a137dda1fb1d2de252a9f2a73329428fe1cf645a",
								"size_bytes": "77"
							}
						},
						"command": ["/bin/bash", "-c", "echo hi > ${ISOLATED_OUTDIR}/out"],
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
	t.Parallel()

	ftt.Run(`ok`, t, func(t *ftt.Test) {
		// prepare input file.
		dir := t.TempDir()
		inputPath := filepath.Join(dir, "input.json")
		err := os.WriteFile(inputPath, []byte(spawnTaskInputJSON), 0600)
		assert.Loosely(t, err, should.BeNil)

		outputPath := filepath.Join(dir, "output.json")
		assert.Loosely(t, runCmd(t, "spawn_task", "-json-input", inputPath, "-json-output", outputPath), should.BeZero)

		results := readTriggerResults(t, outputPath)
		assert.Loosely(t, results.Tasks, should.HaveLength(1))
	})
}
