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

package swarmingimpl

import (
	"bytes"
	"context"
	"testing"

	"github.com/maruel/subcommands"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/swarming/client/swarming"
	"go.chromium.org/luci/swarming/client/swarming/swarmingtest"
	swarmingv2 "go.chromium.org/luci/swarming/proto/api_v2"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

var testSpawnEnv = subcommands.Env{
	swarming.UserEnvVar:   {Value: "test", Exists: true},
	swarming.TaskIDEnvVar: {Value: "293109284abc", Exists: true},
}

func TestSpawnTasksParse(t *testing.T) {
	t.Parallel()

	expectErr := func(argv []string, errLike string) {
		_, code, _, stderr := SubcommandTest(
			context.Background(),
			CmdSpawnTasks,
			append([]string{"-server", "example.com"}, argv...),
			nil, nil,
		)
		So(code, ShouldEqual, 1)
		So(stderr, ShouldContainSubstring, errLike)
	}

	Convey(`Wants -json-input`, t, func() {
		expectErr(nil, "input JSON file is required")
	})
}

func TestProcessTasksStream(t *testing.T) {
	t.Parallel()

	Convey(`Test disallow unknown fields`, t, func() {
		r := bytes.NewReader([]byte(`{"requests": [{"thing": "does not exist"}]}`))
		_, err := processTasksStream(r, testSpawnEnv)
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
		result, err := processTasksStream(r, testSpawnEnv)
		So(err, ShouldBeNil)
		So(result, ShouldHaveLength, 1)
		So(result[0], ShouldResembleProto, &swarmingv2.NewTaskRequest{
			Name:         "foo",
			User:         "test",
			ParentTaskId: "293109284abc",
			Properties: &swarmingv2.TaskProperties{
				Command: []string{"/bin/ls"},
				Outputs: []string{"my_output.bin"},
			},
		})
	})
}

func TestCreateNewTasks(t *testing.T) {
	t.Parallel()

	req := &swarmingv2.NewTaskRequest{Name: "hello!"}
	expectReq := &swarmingv2.TaskRequestResponse{Name: "hello!"}
	ctx := context.Background()

	Convey(`Test fatal response`, t, func() {
		service := &swarmingtest.Client{
			NewTaskMock: func(ctx context.Context, req *swarmingv2.NewTaskRequest) (*swarmingv2.TaskRequestMetadataResponse, error) {
				return nil, status.Errorf(codes.NotFound, "not found")
			},
		}
		_, err := createNewTasks(ctx, service, []*swarmingv2.NewTaskRequest{req})
		So(err, ShouldErrLike, "not found")
	})

	goodService := &swarmingtest.Client{
		NewTaskMock: func(ctx context.Context, req *swarmingv2.NewTaskRequest) (*swarmingv2.TaskRequestMetadataResponse, error) {
			return &swarmingv2.TaskRequestMetadataResponse{
				Request: &swarmingv2.TaskRequestResponse{
					Name: req.Name,
				},
			}, nil
		},
	}

	Convey(`Test single success`, t, func() {
		results, err := createNewTasks(ctx, goodService, []*swarmingv2.NewTaskRequest{req})
		So(err, ShouldBeNil)
		So(results, ShouldHaveLength, 1)
		So(results[0].Request, ShouldResemble, expectReq)
	})

	Convey(`Test many success`, t, func() {
		reqs := make([]*swarmingv2.NewTaskRequest, 0, 12)
		for i := 0; i < 12; i++ {
			reqs = append(reqs, req)
		}
		results, err := createNewTasks(ctx, goodService, reqs)
		So(err, ShouldBeNil)
		So(results, ShouldHaveLength, 12)
		for i := 0; i < 12; i++ {
			So(results[i].Request, ShouldResemble, expectReq)
		}
	})
}
