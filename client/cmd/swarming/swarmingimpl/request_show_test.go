// Copyright 2023 The LUCI Authors.
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

func TestRequestShowParse(t *testing.T) {
	t.Parallel()

	expectErr := func(argv []string, errLike string) {
		_, code, _, stderr := SubcommandTest(
			context.Background(),
			CmdRequestShow,
			append([]string{"-server", "example.com"}, argv...),
			nil, nil,
		)
		So(code, ShouldEqual, 1)
		So(stderr, ShouldContainSubstring, errLike)
	}

	Convey(`Need an arg`, t, func() {
		expectErr(nil, "expecting exactly 1 argument")
	})

	Convey(`At most one arg`, t, func() {
		expectErr([]string{"aaaa", "bbbb"}, "expecting exactly 1 argument")
	})
}

func TestRequestShow(t *testing.T) {
	t.Parallel()

	service := &swarmingtest.Client{
		TaskRequestMock: func(ctx context.Context, s string) (*swarmingv2.TaskRequestResponse, error) {
			So(s, ShouldEqual, "aaaa")
			return &swarmingv2.TaskRequestResponse{
				TaskId:       "aaaa",
				ParentTaskId: "bbbb",
			}, nil
		},
	}

	Convey(`Output is correct`, t, func() {
		_, code, stdout, _ := SubcommandTest(
			context.Background(),
			CmdRequestShow,
			[]string{"-server", "example.com", "aaaa"},
			nil, service,
		)
		So(code, ShouldEqual, 0)
		So(stdout, ShouldEqual, `{
 "task_id": "aaaa",
 "parent_task_id": "bbbb"
}
`)
	})
}
