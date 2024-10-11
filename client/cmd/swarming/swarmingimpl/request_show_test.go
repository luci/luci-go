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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/swarming/client/swarming/swarmingtest"
	swarmingv2 "go.chromium.org/luci/swarming/proto/api_v2"
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
		assert.Loosely(t, code, should.Equal(1))
		assert.Loosely(t, stderr, should.ContainSubstring(errLike))
	}

	ftt.Run(`Need an arg`, t, func(t *ftt.Test) {
		expectErr(nil, "expecting exactly 1 argument")
	})

	ftt.Run(`At most one arg`, t, func(t *ftt.Test) {
		expectErr([]string{"aaaa", "bbbb"}, "expecting exactly 1 argument")
	})
}

func TestRequestShow(t *testing.T) {
	t.Parallel()

	service := &swarmingtest.Client{
		TaskRequestMock: func(ctx context.Context, s string) (*swarmingv2.TaskRequestResponse, error) {
			assert.Loosely(t, s, should.Equal("aaaa"))
			return &swarmingv2.TaskRequestResponse{
				TaskId:       "aaaa",
				ParentTaskId: "bbbb",
			}, nil
		},
	}

	ftt.Run(`Output is correct`, t, func(t *ftt.Test) {
		_, code, stdout, _ := SubcommandTest(
			context.Background(),
			CmdRequestShow,
			[]string{"-server", "example.com", "aaaa"},
			nil, service,
		)
		assert.Loosely(t, code, should.BeZero)
		assert.Loosely(t, stdout, should.Equal(`{
 "task_id": "aaaa",
 "parent_task_id": "bbbb"
}
`))
	})
}
