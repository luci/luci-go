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

func TestTasksParse(t *testing.T) {
	t.Parallel()

	expectErr := func(argv []string, errLike string) {
		_, code, _, stderr := SubcommandTest(
			context.Background(),
			CmdTasks,
			append([]string{"-server", "example.com"}, argv...),
			nil, nil,
		)
		assert.Loosely(t, code, should.Equal(1))
		assert.Loosely(t, stderr, should.ContainSubstring(errLike))
	}

	ftt.Run(`Make sure that Parse fails with zero -limit.`, t, func(t *ftt.Test) {
		expectErr([]string{"-limit", "0"}, "must be positive")
		expectErr([]string{"-limit", "-1"}, "must be positive")
	})

	ftt.Run(`Make sure that Parse requires positive -start with -count.`, t, func(t *ftt.Test) {
		expectErr([]string{"-count"}, "provide -start")
		expectErr([]string{"-count", "-start", "0"}, "provide -start")
		expectErr([]string{"-count", "-start", "-1"}, "provide -start")
	})

	ftt.Run(`Make sure that Parse forbids -field or -limit to be used with -count.`, t, func(t *ftt.Test) {
		expectErr([]string{"-count", "-start", "1337", "-field", "items/task_id"}, "-field cannot")
		expectErr([]string{"-count", "-start", "1337", "-limit", "100"}, "-limit cannot")
	})
}

func TestTasksOutput(t *testing.T) {
	t.Parallel()

	service := &swarmingtest.Client{
		ListTasksMock: func(ctx context.Context, i int32, f float64, sq swarmingv2.StateQuery, s []string) ([]*swarmingv2.TaskResultResponse, error) {
			assert.Loosely(t, s, should.Resemble([]string{"t:1", "t:2"}))
			assert.Loosely(t, swarmingv2.StateQuery_QUERY_PENDING, should.Equal(sq))
			return []*swarmingv2.TaskResultResponse{
				{TaskId: "task1"},
				{TaskId: "task2"},
			}, nil
		},
		CountTasksMock: func(ctx context.Context, f float64, sq swarmingv2.StateQuery, s []string) (*swarmingv2.TasksCount, error) {
			return &swarmingv2.TasksCount{
				Count: 123,
			}, nil
		},
	}

	ftt.Run(`Listing tasks`, t, func(t *ftt.Test) {
		_, code, stdout, _ := SubcommandTest(
			context.Background(),
			CmdTasks,
			[]string{"-server", "example.com", "-state", "pending", "-tag", "t:1", "-tag", "t:2"},
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

	ftt.Run(`Counting tasks`, t, func(t *ftt.Test) {
		_, code, stdout, _ := SubcommandTest(
			context.Background(),
			CmdTasks,
			[]string{"-server", "example.com", "-count", "-start", "12345"},
			nil, service,
		)
		assert.Loosely(t, code, should.BeZero)
		assert.Loosely(t, stdout, should.Equal(`{
 "count": 123
}
`))
	})
}
