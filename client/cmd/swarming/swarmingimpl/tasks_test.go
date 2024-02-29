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

	"go.chromium.org/luci/swarming/client/swarming/swarmingtest"
	swarmingv2 "go.chromium.org/luci/swarming/proto/api_v2"

	. "github.com/smartystreets/goconvey/convey"
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
		So(code, ShouldEqual, 1)
		So(stderr, ShouldContainSubstring, errLike)
	}

	Convey(`Make sure that Parse fails with zero -limit.`, t, func() {
		expectErr([]string{"-limit", "0"}, "must be positive")
		expectErr([]string{"-limit", "-1"}, "must be positive")
	})

	Convey(`Make sure that Parse requires positive -start with -count.`, t, func() {
		expectErr([]string{"-count"}, "provide -start")
		expectErr([]string{"-count", "-start", "0"}, "provide -start")
		expectErr([]string{"-count", "-start", "-1"}, "provide -start")
	})

	Convey(`Make sure that Parse forbids -field or -limit to be used with -count.`, t, func() {
		expectErr([]string{"-count", "-start", "1337", "-field", "items/task_id"}, "-field cannot")
		expectErr([]string{"-count", "-start", "1337", "-limit", "100"}, "-limit cannot")
	})
}

func TestTasksOutput(t *testing.T) {
	t.Parallel()

	service := &swarmingtest.Client{
		ListTasksMock: func(ctx context.Context, i int32, f float64, sq swarmingv2.StateQuery, s []string) ([]*swarmingv2.TaskResultResponse, error) {
			So(s, ShouldEqual, []string{"t:1", "t:2"})
			So(swarmingv2.StateQuery_QUERY_PENDING, ShouldEqual, sq)
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

	Convey(`Listing tasks`, t, func() {
		_, code, stdout, _ := SubcommandTest(
			context.Background(),
			CmdTasks,
			[]string{"-server", "example.com", "-state", "pending", "-tag", "t:1", "-tag", "t:2"},
			nil, service,
		)
		So(code, ShouldEqual, 0)
		So(stdout, ShouldEqual, `[
 {
  "task_id": "task1"
 },
 {
  "task_id": "task2"
 }
]
`)
	})

	Convey(`Counting tasks`, t, func() {
		_, code, stdout, _ := SubcommandTest(
			context.Background(),
			CmdTasks,
			[]string{"-server", "example.com", "-count", "-start", "12345"},
			nil, service,
		)
		So(code, ShouldEqual, 0)
		So(stdout, ShouldEqual, `{
 "count": 123
}
`)
	})
}
