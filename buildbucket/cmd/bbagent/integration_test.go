// Copyright 2021 The LUCI Authors.
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
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/logging"
	ldMemory "go.chromium.org/luci/logdog/client/butler/output/memory"
	"go.chromium.org/luci/luciexe/build"
)

// The integration tests in this file are pretty heavy handed:
//   * they impersonate the buildbucket UpdateBuild API
//   * they fake a logdog sink
//   * they run the real bbagent program in this environment (sans
//     os.Exit calls)
//   * with a target of this test executable (with a special env
//     var) as the user program.

// bbagentTestCases describes all the test cases.
//
// Each test case has three parts:
//   * A globally unique name; used in init() to select this test case to
//     act as main() when $BBAGENT_TEST_PROG is set.
//   * The main() function of the target executable to be run by bbagent.
//     This would be equivalent to the "recipe" in some recipe based builder.
//   * The test case function which actually invokes bbagent and then asserts
//     the outcome.
//
// See TestBBAgent for how these are used.
var bbagentTestCases = []bbagentTestCase{
	declareBBAgentTestCase("simpleSuccess", func() {
		build.Main(nil, nil, nil, func(ctx context.Context, args []string, st *build.State) error {
			return nil
		})
	}, func(runBbagent func(*bbpb.Build) (int, *ldMemory.Output), getBuild func() *bbpb.Build) {
		retcode, _ := runBbagent(getBuild())
		So(retcode, ShouldEqual, 0)
		finalBuild := getBuild()
		So(finalBuild.Status, ShouldEqual, bbpb.Status_SUCCESS)
	}),

	declareBBAgentTestCase("stepsAndLogs", func() {
		build.Main(nil, nil, nil, func(ctx context.Context, args []string, st *build.State) error {
			logging.Infof(ctx, "base log")

			step1, ctx1 := build.StartStep(ctx, "a step")
			defer step1.End(nil)

			logging.Infof(ctx1, "step1 log")
			fmt.Fprintln(step1.Log("some log"), "this is cool")

			step2, ctx2 := build.StartStep(ctx, "another step")
			defer step2.End(nil)

			logging.Infof(ctx2, "step2 log")
			fmt.Fprintln(step2.Log("other log"), "this is neat")
			return nil
		})
	}, func(runBbagent func(*bbpb.Build) (int, *ldMemory.Output), getBuild func() *bbpb.Build) {
		retcode, logs := runBbagent(getBuild())
		So(retcode, ShouldEqual, 0)

		finalBuild := getBuild()
		So(finalBuild.Status, ShouldEqual, bbpb.Status_SUCCESS)
		So(finalBuild.Steps[0].Name, ShouldEqual, "a step")
		So(finalBuild.Steps[1].Name, ShouldEqual, "another step")
		So(logs.GetStream("", "u/stderr").LastData(), ShouldContainSubstring, "base log")
		So(logs.GetStream("", "u/step/0/log/0").LastData(), ShouldContainSubstring, "step1 log")
		So(logs.GetStream("", "u/step/0/log/1").LastData(), ShouldContainSubstring, "this is cool")
		So(logs.GetStream("", "u/step/1/log/0").LastData(), ShouldContainSubstring, "step2 log")
		So(logs.GetStream("", "u/step/1/log/1").LastData(), ShouldContainSubstring, "this is neat")
	}),
}

func TestBBAgent(t *testing.T) {
	addr, shutdown, scheduleBuild, getBuild := startFakeBuildbucketBuildService()
	defer shutdown()

	Convey(`bbagent`, t, func() {
		for _, tc := range bbagentTestCases {
			Convey(tc.name, func() {
				bid := scheduleBuild()

				run := func(input *bbpb.Build) (int, *ldMemory.Output) {
					return runBbagent(addr, input, tc)
				}

				get := func() *bbpb.Build {
					return getBuild(bid)
				}

				tc.testFn(run, get)
			})
		}
	})
}
