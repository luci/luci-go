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
	. "github.com/smartystreets/goconvey/convey"
	bbpb "go.chromium.org/luci/buildbucket/proto"
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
		"https://logs.chromium.org/logs/ninja_log":  "ninja_log",
		"https://logs.chromium.org/logs/stdout_log": "stdout_log",
	})
	res := &bbpb.Build{
		Steps: []*bbpb.Step{
			{
				Name: "compile",
				Logs: []*bbpb.Log{
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

	Convey("GetCompileLog", t, func() {
		ninjaLogJson := map[string]interface{}{
			"failures": []interface{}{
				map[string]interface{}{
					"dependencies": []string{"d1", "d2"},
					"output":       "/opt/s/w/ir/cache/goma/client/gomacc blah blah...",
					"output_nodes": []string{"n1", "n2"},
					"rule":         "CXX",
				},
			},
		}
		ninjaLogStr, err := json.Marshal(ninjaLogJson)
		So(err, ShouldBeNil)

		c = logdog.MockClientContext(c, map[string]string{
			"https://logs.chromium.org/logs/ninja_log":  string(ninjaLogStr),
			"https://logs.chromium.org/logs/stdout_log": "stdout_log",
		})
		logs, err := GetCompileLogs(c, 12345)
		So(err, ShouldBeNil)
		So(*logs, ShouldResemble, model.CompileLogs{
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
		})
	})
	Convey("GetCompileLog failed", t, func() {
		c = logdog.MockClientContext(c, map[string]string{})
		_, err := GetCompileLogs(c, 12345)
		So(err, ShouldNotBeNil)
	})
}

func TestGetFailedTargets(t *testing.T) {
	t.Parallel()

	Convey("No Ninja log", t, func() {
		compileLogs := &model.CompileLogs{}
		So(GetFailedTargets(compileLogs), ShouldResemble, []string{})
	})

	Convey("Have Ninja log", t, func() {
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
		So(GetFailedTargets(compileLogs), ShouldResemble, []string{"node1", "node2", "node3", "node4"})
	})
}
