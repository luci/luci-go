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

package compilelog

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/golang/mock/gomock"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"

	"go.chromium.org/luci/bisection/internal/buildbucket"
	"go.chromium.org/luci/bisection/internal/logdog"
	"go.chromium.org/luci/bisection/model"
)

func TestGetCompileLogs(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())

	// Setup mock
	ctl := gomock.NewController(t)
	defer ctl.Finish()
	mc := buildbucket.NewMockedClient(c, ctl)
	c = mc.Ctx
	c = logdog.MockClientContext(c, map[string]string{
		"https://logs.chromium.org/logs/ninja_log":       "ninja_log",
		"https://logs.chromium.org/logs/stdout_log":      "stdout_log",
		"https://logs.chromium.org/logs/failure_summary": "failure_summary_log",
	})
	res := &bbpb.Build{
		Steps: []*bbpb.Step{
			{
				Name: "compile",
				Logs: []*bbpb.Log{
					{
						Name:    "json.output[failure_summary]",
						ViewUrl: "https://logs.chromium.org/logs/failure_summary",
					},
					{
						Name:    "json.output[ninja_info]",
						ViewUrl: "https://logs.chromium.org/logs/ninja_log",
					},
					{
						Name:    "stdout",
						ViewUrl: "https://logs.chromium.org/logs/stdout_log",
					},
				},
			},
		},
	}
	mc.Client.EXPECT().GetBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(res, nil).AnyTimes()

	ftt.Run("GetCompileLog", t, func(t *ftt.Test) {
		ninjaLogJson := map[string]any{
			"failures": []any{
				map[string]any{
					"dependencies": []string{"d1", "d2"},
					"output":       "/opt/s/w/ir/cache/goma/client/gomacc blah blah...",
					"output_nodes": []string{"n1", "n2"},
					"rule":         "CXX",
				},
			},
		}
		ninjaLogStr, err := json.Marshal(ninjaLogJson)
		assert.Loosely(t, err, should.BeNil)

		c = logdog.MockClientContext(c, map[string]string{
			"https://logs.chromium.org/logs/failure_summary": "failure_summary_log",
			"https://logs.chromium.org/logs/ninja_log":       string(ninjaLogStr),
			"https://logs.chromium.org/logs/stdout_log":      "stdout_log",
		})
		logs, err := GetCompileLogs(c, 12345)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, *logs, should.Match(model.CompileLogs{
			FailureSummaryLog: "failure_summary_log",
			NinjaLog: &model.NinjaLog{
				Failures: []*model.NinjaLogFailure{
					{
						Dependencies: []string{"d1", "d2"},
						Output:       "/opt/s/w/ir/cache/goma/client/gomacc blah blah...",
						OutputNodes:  []string{"n1", "n2"},
						Rule:         "CXX",
					},
				},
			},
			StdOutLog: "stdout_log",
		}))
	})
	ftt.Run("GetCompileLog failed", t, func(t *ftt.Test) {
		c = logdog.MockClientContext(c, map[string]string{})
		_, err := GetCompileLogs(c, 12345)
		assert.Loosely(t, err, should.NotBeNil)
	})
}

func TestGetFailedTargets(t *testing.T) {
	t.Parallel()

	ftt.Run("No Ninja log", t, func(t *ftt.Test) {
		compileLogs := &model.CompileLogs{}
		assert.Loosely(t, GetFailedTargets(compileLogs), should.Match([]string{}))
	})

	ftt.Run("Have Ninja log", t, func(t *ftt.Test) {
		compileLogs := &model.CompileLogs{
			NinjaLog: &model.NinjaLog{
				Failures: []*model.NinjaLogFailure{
					{
						OutputNodes: []string{"node1", "node2"},
					},
					{
						OutputNodes: []string{"node3", "node4"},
					},
				},
			},
		}
		assert.Loosely(t, GetFailedTargets(compileLogs), should.Match([]string{"node1", "node2", "node3", "node4"}))
	})
}
