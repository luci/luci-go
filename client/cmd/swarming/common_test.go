// Copyright 2019 The LUCI Authors.
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
	"context"
	"fmt"
	"regexp"
	"testing"
	"time"

	googleapi "google.golang.org/api/googleapi"

	"go.chromium.org/luci/common/api/swarming/swarming/v1"
	. "go.chromium.org/luci/common/testing/assertions"

	. "github.com/smartystreets/goconvey/convey"
)

var InvocationUUIDRegexp = regexp.MustCompile(`^InvocationUUID:[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89aAbB][a-f0-9]{3}-[a-f0-9]{12}$`)
var RPCUUIDRegexp = regexp.MustCompile(`^RPCUUID:[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89aAbB][a-f0-9]{3}-[a-f0-9]{12}$`)

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
		invocationTag, err := addInvocationUUIDTags(requests...)
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
		rpcTags, err := addRPCUUIDTags(requests...)
		So(err, ShouldBeNil)
		So(RPCUUIDRegexp.MatchString(rpcTags[0]), ShouldResemble, true)
		So(RPCUUIDRegexp.MatchString(rpcTags[1]), ShouldResemble, true)
		So(requests[0].Tags[0], ShouldResemble, rpcTags[0])
		So(requests[1].Tags[0], ShouldResemble, rpcTags[1])
		So(requests[0].Tags[0], ShouldNotResemble, requests[1].Tags[0])
	})
}

func mockCountTasks(c context.Context, start float64, tags ...string) (*swarming.SwarmingRpcsTasksCount, error) {
	for _, tag := range tags {
		if !InvocationUUIDRegexp.MatchString(tag) {
			return nil, fmt.Errorf("Invalid tag %s", tag)
		}
	}
	return &swarming.SwarmingRpcsTasksCount{
		Count: 2,
	}, nil
}

func mockListTasks(c context.Context, start float64, tags ...string) (*swarming.SwarmingRpcsTaskList, error) {
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
	createStart := float64(time.Now().Unix())
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
		countTasks: mockCountTasks,
		listTasks:  mockListTasks,
		cancelTask: func(c context.Context, taskID string, req *swarming.SwarmingRpcsTaskCancelRequest) (*swarming.SwarmingRpcsCancelResponse, error) {
			return &swarming.SwarmingRpcsCancelResponse{
				Ok: true,
			}, nil
		},
	}

	Convey(`Test success`, t, func() {
		err := cancelExtraTasks(c, &duplicatingService, createStart, invocationTag, results)
		So(err, ShouldBeNil)
	})

	badCountService := testService{
		countTasks: func(c context.Context, start float64, tags ...string) (*swarming.SwarmingRpcsTasksCount, error) {
			return nil, &googleapi.Error{Code: 404}
		},
	}

	Convey(`Test fatal count response`, t, func() {
		err := cancelExtraTasks(c, &badCountService, createStart, invocationTag, results)
		So(err, ShouldErrLike, "404")
	})

	badListService := testService{
		countTasks: mockCountTasks,
		listTasks: func(c context.Context, start float64, tags ...string) (*swarming.SwarmingRpcsTaskList, error) {
			return nil, &googleapi.Error{Code: 404}
		},
	}

	Convey(`Test fatal list response`, t, func() {
		err := cancelExtraTasks(c, &badListService, createStart, invocationTag, results)
		So(err, ShouldErrLike, "404")
	})
}
