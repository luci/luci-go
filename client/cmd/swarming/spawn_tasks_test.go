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

package main

import (
	"bytes"
	"context"
	"os"
	"testing"

	googleapi "google.golang.org/api/googleapi"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/api/swarming/swarming/v1"
	. "go.chromium.org/luci/common/testing/assertions"

	. "github.com/smartystreets/goconvey/convey"
)

func mockEnv(env map[string]string) func() {
	unset := make([]string, 0, len(env))
	fixup := make(map[string]string)
	for k, v := range env {
		if orig, ok := os.LookupEnv(k); ok {
			fixup[k] = orig
		} else {
			unset = append(unset, k)
		}
		os.Setenv(k, v)
	}
	return func() {
		for _, k := range unset {
			os.Unsetenv(k)
		}
		for k, v := range fixup {
			os.Setenv(k, v)
		}
	}
}

func TestSpawnTasksParse_NoArgs(t *testing.T) {
	Convey(`Make sure that Parse works with no arguments.`, t, func() {
		c := spawnTasksRun{}
		c.Init(auth.Options{})

		err := c.Parse([]string{})
		So(err, ShouldErrLike, "must provide -server")
	})
}

func TestSpawnTasksParse_NoInput(t *testing.T) {
	Convey(`Make sure that Parse handles no input JSON given.`, t, func() {
		c := spawnTasksRun{}
		c.Init(auth.Options{})

		err := c.GetFlags().Parse([]string{"-server", "http://localhost:9050"})

		err = c.Parse([]string{})
		So(err, ShouldErrLike, "input JSON")
	})
}

func TestProcessTasksStream(t *testing.T) {
	Convey(`Test disallow unknown fields`, t, func() {
		r := bytes.NewReader([]byte(`{"requests": [{"thing": "does not exist"}]}`))
		_, err := processTasksStream(r)
		So(err, ShouldNotBeNil)
	})

	Convey(`Test success`, t, func() {
		r := bytes.NewReader([]byte(`{
			"requests": [
				{
					"name": "foo",
					"properties": {
						"command": ["/bin/ls"],
						"outputs": ["my_output.bin"]
					}
				}
			]
		}`))
		// Set environment variables to ensure they get picked up.
		defer mockEnv(map[string]string{
			"USER":             "test",
			"SWARMING_TASK_ID": "293109284abc",
		})()
		result, err := processTasksStream(r)
		So(err, ShouldBeNil)
		So(result, ShouldHaveLength, 1)
		So(result[0], ShouldResemble, &swarming.SwarmingRpcsNewTaskRequest{
			Name:         "foo",
			User:         "test",
			ParentTaskId: "293109284abc",
			Properties: &swarming.SwarmingRpcsTaskProperties{
				Command: []string{"/bin/ls"},
				Outputs: []string{"my_output.bin"},
			},
		})
	})
}

func TestCreateNewTasks(t *testing.T) {
	t.Parallel()

	req := &swarming.SwarmingRpcsNewTaskRequest{Name: "hello!"}
	expectReq := &swarming.SwarmingRpcsTaskRequest{Name: "hello!"}
	c := context.Background()

	Convey(`Test fatal response`, t, func() {
		service := &testService{
			newTask: func(c context.Context, req *swarming.SwarmingRpcsNewTaskRequest) (*swarming.SwarmingRpcsTaskRequestMetadata, error) {
				return nil, &googleapi.Error{Code: 404}
			},
		}
		_, err := createNewTasks(c, service, []*swarming.SwarmingRpcsNewTaskRequest{req})
		So(err, ShouldErrLike, "404")
	})

	goodService := &testService{
		newTask: func(c context.Context, req *swarming.SwarmingRpcsNewTaskRequest) (*swarming.SwarmingRpcsTaskRequestMetadata, error) {
			return &swarming.SwarmingRpcsTaskRequestMetadata{
				Request: &swarming.SwarmingRpcsTaskRequest{
					Name: req.Name,
				},
			}, nil
		},
	}

	Convey(`Test single success`, t, func() {
		results, err := createNewTasks(c, goodService, []*swarming.SwarmingRpcsNewTaskRequest{req})
		So(err, ShouldBeNil)
		So(results, ShouldHaveLength, 1)
		So(results[0].Request, ShouldResemble, expectReq)
	})

	Convey(`Test many success`, t, func() {
		reqs := make([]*swarming.SwarmingRpcsNewTaskRequest, 0, 12)
		for i := 0; i < 12; i++ {
			reqs = append(reqs, req)
		}
		results, err := createNewTasks(c, goodService, reqs)
		So(err, ShouldBeNil)
		So(results, ShouldHaveLength, 12)
		for i := 0; i < 12; i++ {
			So(results[i].Request, ShouldResemble, expectReq)
		}
	})
}
