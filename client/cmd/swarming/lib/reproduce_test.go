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

package lib

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/luci/common/api/swarming/swarming/v1"
	. "go.chromium.org/luci/common/testing/assertions"
)

func init() {
	// So that this test works on swarming!
	os.Unsetenv(ServerEnvVar)
	os.Unsetenv(TaskIDEnvVar)
}

func TestReproduceParse_NoArgs(t *testing.T) {
	t.Parallel()
	Convey(`Make sure Parse works with no arguments.`, t, func() {
		c := reproduceRun{}
		c.init(&testAuthFlags{})

		err := c.parse([]string(nil))
		So(err, ShouldErrLike, "must provide -server")
	})
}

func TestReproduceParse_NoTaskID(t *testing.T) {
	t.Parallel()
	Convey(`Make sure Parse works with with no task ID.`, t, func() {
		c := reproduceRun{}
		c.init(&testAuthFlags{})

		err := c.GetFlags().Parse([]string{"-server", "http://localhost:9050"})

		err = c.parse([]string(nil))
		So(err, ShouldErrLike, "must specify exactly one task id.")
	})
}

func TestCreateTaskRequestCommand(t *testing.T) {
	t.Parallel()
	Convey(`Make sure we can create the correct Cmd from a fetched task's properties.`, t, func() {
		c := reproduceRun{}
		c.init(&testAuthFlags{})
		// Use TempDir, which creates a temp directory, to return a unique directory name
		// that createTaskRequestCommand() will remove and recreate (via prepareDir()).
		c.work = t.TempDir()
		ctx := context.Background()

		expectedBaseEnv := os.Environ()
		os.Setenv("removeKey", "removeValue")
		defer os.Unsetenv("removeKey")
		os.Setenv("replaceKey", "oldValue")
		defer os.Unsetenv("replaceKey")

		service := &testService{
			getTaskRequest: func(_ context.Context, _ string) (*swarming.SwarmingRpcsTaskRequest, error) {
				return &swarming.SwarmingRpcsTaskRequest{
					TaskSlices: []*swarming.SwarmingRpcsTaskSlice{
						&swarming.SwarmingRpcsTaskSlice{
							Properties: &swarming.SwarmingRpcsTaskProperties{
								Command: []string{"rbd", "stream", "-test-id-prefix", "chicken://cow_cow/"},
							},
						},
						&swarming.SwarmingRpcsTaskSlice{
							Properties: &swarming.SwarmingRpcsTaskProperties{
								Command: []string{"rbd", "stream", "-test-id-prefix", "chicken://chicken_chicken/"},
								Env: []*swarming.SwarmingRpcsStringPair{
									&swarming.SwarmingRpcsStringPair{
										Key:   "key",
										Value: "value1",
									},
									&swarming.SwarmingRpcsStringPair{
										Key:   "replaceKey",
										Value: "value2",
									},
									&swarming.SwarmingRpcsStringPair{
										Key:   "removeKey",
										Value: "",
									},
								},
							},
						},
					},
				}, nil
			},
		}
		cmd, err := c.createTaskRequestCommand(ctx, "task-123", service)
		So(err, ShouldBeNil)
		expected := exec.CommandContext(ctx, "rbd", "stream", "-test-id-prefix", "chicken://chicken_chicken/")
		expected.Dir = c.work

		expected.Env = append(expectedBaseEnv, []string{"key=value1", "replaceKey=value2", "SWARMING_BOT_ID=reproduce", "SWARMING_TASK_ID=reproduce"}...)
		sort.Strings(cmd.Env)
		sort.Strings(expected.Env)
		So(cmd, ShouldResemble, expected)
	})
}

func TestReproduceTaskRequestCommand(t *testing.T) {
	t.Parallel()
	Convey(`Make sure we can execute commands.`, t, func() {
		c := reproduceRun{}
		c.init(&testAuthFlags{})
		ctx := context.Background()
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.CommandContext(ctx, "cmd", "/c", "echo", "chicken")
		} else {
			cmd = exec.CommandContext(ctx, "echo", "chicken")
		}

		var stdBuffer bytes.Buffer
		cmd.Stdout = &stdBuffer
		cmd.Stderr = &stdBuffer
		err := c.executeTaskRequestCommand(cmd)
		So(err, ShouldBeNil)
		if runtime.GOOS == "windows" {
			So(stdBuffer.String(), ShouldEqual, "chicken\r\n")
		} else {
			So(stdBuffer.String(), ShouldEqual, "chicken\n")
		}

	})
}
