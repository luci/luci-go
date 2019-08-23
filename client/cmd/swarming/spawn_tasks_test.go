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
	"fmt"
	"os"
	"regexp"
	"testing"

	googleapi "google.golang.org/api/googleapi"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/api/swarming/swarming/v1"
	. "go.chromium.org/luci/common/testing/assertions"

	. "github.com/smartystreets/goconvey/convey"
)

var InvocationUUIDRegexp, RPCUUIDRegexp *regexp.Regexp

func init() {
	InvocationUUIDRegexp = regexp.MustCompile(`^InvocationUUID:[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89aAbB][a-f0-9]{3}-[a-f0-9]{12}$`)
	RPCUUIDRegexp = regexp.MustCompile(`^RPCUUID:[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89aAbB][a-f0-9]{3}-[a-f0-9]{12}$`)
}

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

func TestAddInvocationUUIDTags(t *testing.T) {
	t.Parallel()

	requests := []*swarming.SwarmingRpcsNewTaskRequest{
		{
			Name: "foo",
		},
		{
			Name: "bar",
		},
	}
	Convey(`Test success`, t, func() {
		invocationTag, err := addInvocationUUIDTags(requests)
		So(err, ShouldBeNil)
		So(InvocationUUIDRegexp.MatchString(requests[0].Tags[0]), ShouldResemble, true)
		So(requests[0].Tags[0], ShouldResemble, invocationTag)
		So(requests[1].Tags[0], ShouldResemble, invocationTag)
	})
}

func TestAddRPCUUIDTags(t *testing.T) {
	t.Parallel()

	requests := []*swarming.SwarmingRpcsNewTaskRequest{
		{
			Name: "foo",
		},
		{
			Name: "bar",
		},
	}
	Convey(`Test success`, t, func() {
		rpcTags, err := addRPCUUIDTags(requests)
		So(err, ShouldBeNil)
		So(RPCUUIDRegexp.MatchString(rpcTags[0]), ShouldResemble, true)
		So(RPCUUIDRegexp.MatchString(rpcTags[1]), ShouldResemble, true)
		So(requests[0].Tags[0], ShouldResemble, rpcTags[0])
		So(requests[1].Tags[0], ShouldResemble, rpcTags[1])
		So(requests[0].Tags[0], ShouldNotResemble, requests[1].Tags[0])
	})
}

func mockCountTasksByTags(c context.Context, tags ...string) (*swarming.SwarmingRpcsTasksCount, error) {
	for _, tag := range tags {
		if !InvocationUUIDRegexp.MatchString(tag) {
			return nil, fmt.Errorf("Invalid tag %s", tag)
		}
	}
	return &swarming.SwarmingRpcsTasksCount{
		Count: 2,
	}, nil
}

func mockListTasksByTags(c context.Context, tags ...string) (*swarming.SwarmingRpcsTaskList, error) {
	for _, tag := range tags {
		if !InvocationUUIDRegexp.MatchString(tag) {
			return nil, fmt.Errorf("Invalid tag %s", tag)
		}
	}
	return &swarming.SwarmingRpcsTaskList{
		Items: []*swarming.SwarmingRpcsTaskResult{
			{
				TaskId: "taskID1",
			},
			{
				TaskId: "taskID2",
			},
		},
	}, nil
}

func TestCancelExtraTasks(t *testing.T) {
	t.Parallel()

	invocationTag := "InvocationUUID:abcd1234-abcd-4abc-8abc-abcdef123456"
	rpcTag := "RPCUUID:dcba4321-dcba-4cba-8cba-fedcba654321"
	results := []*swarming.SwarmingRpcsTaskRequestMetadata{
		{
			Request: &swarming.SwarmingRpcsTaskRequest{
				Name: "task1",
				Tags: []string{invocationTag, rpcTag},
			},
		},
	}
	c := context.Background()

	duplicatingService := testService{
		countTasksByTags: mockCountTasksByTags,
		listTasksByTags:  mockListTasksByTags,
		cancelTask: func(c context.Context, taskID string, req *swarming.SwarmingRpcsTaskCancelRequest) (*swarming.SwarmingRpcsCancelResponse, error) {
			return &swarming.SwarmingRpcsCancelResponse{
				Ok: true,
			}, nil
		},
	}

	Convey(`Test success`, t, func() {
		err := cancelExtraTasks(c, &duplicatingService, invocationTag, results)
		So(err, ShouldBeNil)
	})

	badCountService := testService{
		countTasksByTags: func(c context.Context, tags ...string) (*swarming.SwarmingRpcsTasksCount, error) {
			return nil, &googleapi.Error{Code: 404}
		},
	}

	Convey(`Test fatal count response`, t, func() {
		err := cancelExtraTasks(c, &badCountService, invocationTag, results)
		So(err, ShouldErrLike, "404")
	})

	badListService := testService{
		countTasksByTags: mockCountTasksByTags,
		listTasksByTags: func(c context.Context, tags ...string) (*swarming.SwarmingRpcsTaskList, error) {
			return nil, &googleapi.Error{Code: 404}
		},
	}

	Convey(`Test fatal list response`, t, func() {
		err := cancelExtraTasks(c, &badListService, invocationTag, results)
		So(err, ShouldErrLike, "404")
	})

}
