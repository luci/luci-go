// Copyright 2022 The LUCI Authors.
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
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/swarming/client/swarming/swarmingtest"
	swarmingv2 "go.chromium.org/luci/swarming/proto/api_v2"
)

func TestBotTasksParse(t *testing.T) {
	t.Parallel()

	expectErr := func(argv []string, errLike string) {
		_, code, _, stderr := SubcommandTest(
			context.Background(),
			CmdBotTasks,
			append([]string{"-server", "example.com"}, argv...),
			nil, nil,
		)
		assert.Loosely(t, code, should.Equal(1))
		assert.Loosely(t, stderr, should.ContainSubstring(errLike))
	}

	ftt.Run(`Make sure that Parse fails with invalid -id.`, t, func(t *ftt.Test) {
		expectErr([]string{"-id", ""}, "non-empty -id")
	})

	ftt.Run(`Make sure that Parse fails with negative -limit.`, t, func(t *ftt.Test) {
		expectErr([]string{"-id", "device_id", "-limit", "-1"}, "invalid -limit")
	})

	ftt.Run(`Make sure that Parse fails with invalid -state.`, t, func(t *ftt.Test) {
		expectErr([]string{"-id", "device_id", "-state", "invalid_state"}, "Invalid state invalid_state")
	})
}

func TestListBotTasksOutput(t *testing.T) {
	t.Parallel()

	botID := "bot1"

	expectedTasks := []*swarmingv2.TaskResultResponse{
		{TaskId: "task1"},
		{TaskId: "task2"},
	}

	service := &swarmingtest.Client{
		ListBotTasksMock: func(ctx context.Context, s string, i int32, f float64, sq swarmingv2.StateQuery) ([]*swarmingv2.TaskResultResponse, error) {
			assert.Loosely(t, s, should.Equal(botID))
			return expectedTasks, nil
		},
	}

	ftt.Run(`Expected tasks are outputted`, t, func(t *ftt.Test) {
		_, code, stdout, _ := SubcommandTest(
			context.Background(),
			CmdBotTasks,
			[]string{"-server", "example.com", "-id", botID},
			nil, service,
		)
		assert.Loosely(t, code, should.BeZero)
		assert.Loosely(t, stdout, should.Equal(`[
 {
  "task_id": "task1"
 },
 {
  "task_id": "task2"
 }
]
`))
	})
}
