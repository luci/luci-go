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
	"net/url"
	"testing"

	"github.com/maruel/subcommands"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/lhttp"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/swarming/client/swarming"
	"go.chromium.org/luci/swarming/client/swarming/swarmingtest"
	swarmingv2 "go.chromium.org/luci/swarming/proto/api_v2"
)

var testSpawnEnv = subcommands.Env{
	swarming.UserEnvVar:   {Value: "test", Exists: true},
	swarming.TaskIDEnvVar: {Value: "293109284abc", Exists: true},
	swarming.ServerEnvVar: {Value: "https://example.com", Exists: true},
}

func MustParseHostURL(t *testing.T, url string) *url.URL {
	parsedURL, err := lhttp.ParseHostURL(url)
	if err != nil {
		t.Fatal(err)
	}
	return parsedURL
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
		assert.Loosely(t, code, should.Equal(1))
		assert.Loosely(t, stderr, should.ContainSubstring(errLike))
	}

	ftt.Run(`Wants -json-input`, t, func(t *ftt.Test) {
		expectErr(nil, "input JSON file is required")
	})
}

func TestProcessTasksStream(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	serverURL := MustParseHostURL(t, "https://example.com")

	ftt.Run(`Test disallow unknown fields`, t, func(t *ftt.Test) {
		r := bytes.NewReader([]byte(`{"requests": [{"thing": "does not exist"}]}`))
		_, err := processTasksStream(ctx, r, testSpawnEnv, serverURL)
		assert.Loosely(t, err, should.NotBeNil)
	})

	ftt.Run(`Test success`, t, func(t *ftt.Test) {
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
		result, err := processTasksStream(ctx, r, testSpawnEnv, serverURL)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, result, should.HaveLength(1))
		assert.Loosely(t, result[0], should.Match(&swarmingv2.NewTaskRequest{
			Name:         "foo",
			User:         "test",
			ParentTaskId: "293109284abc",
			Properties: &swarmingv2.TaskProperties{
				Command: []string{"/bin/ls"},
				Outputs: []string{"my_output.bin"},
			},
		}))
	})

	ftt.Run(`Test with different server url`, t, func(t *ftt.Test) {
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

		result, err := processTasksStream(ctx, r, testSpawnEnv, MustParseHostURL(t.T, "https://other-server.com"))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, result, should.HaveLength(1))
		assert.Loosely(t, result[0].ParentTaskId, should.BeEmpty)
	})

	ftt.Run(`Test with server env var unset`, t, func(t *ftt.Test) {
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

		envWithoutServer := subcommands.Env{
			swarming.UserEnvVar:   {Value: "test", Exists: true},
			swarming.TaskIDEnvVar: {Value: "293109284abc", Exists: true},
		}

		result, err := processTasksStream(ctx, r, envWithoutServer, serverURL)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, result, should.HaveLength(1))
		assert.Loosely(t, result[0].ParentTaskId, should.BeEmpty)
	})
}

func TestCreateNewTasks(t *testing.T) {
	t.Parallel()

	req := &swarmingv2.NewTaskRequest{Name: "hello!"}
	expectReq := &swarmingv2.TaskRequestResponse{Name: "hello!"}
	ctx := context.Background()

	ftt.Run(`Test fatal response`, t, func(t *ftt.Test) {
		service := &swarmingtest.Client{
			NewTaskMock: func(ctx context.Context, req *swarmingv2.NewTaskRequest) (*swarmingv2.TaskRequestMetadataResponse, error) {
				return nil, status.Errorf(codes.NotFound, "not found")
			},
		}
		_, err := createNewTasks(ctx, service, []*swarmingv2.NewTaskRequest{req})
		assert.Loosely(t, err, should.ErrLike("not found"))
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

	ftt.Run(`Test single success`, t, func(t *ftt.Test) {
		results, err := createNewTasks(ctx, goodService, []*swarmingv2.NewTaskRequest{req})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, results, should.HaveLength(1))
		assert.Loosely(t, results[0].Request, should.Match(expectReq))
	})

	ftt.Run(`Test many success`, t, func(t *ftt.Test) {
		reqs := make([]*swarmingv2.NewTaskRequest, 0, 12)
		for i := 0; i < 12; i++ {
			reqs = append(reqs, req)
		}
		results, err := createNewTasks(ctx, goodService, reqs)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, results, should.HaveLength(12))
		for i := 0; i < 12; i++ {
			assert.Loosely(t, results[i].Request, should.Match(expectReq))
		}
	})
}
