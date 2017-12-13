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

package main

import (
	"net/http"
	"testing"
	"time"

	"golang.org/x/net/context"

	googleapi "google.golang.org/api/googleapi"

	"go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/auth"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	. "go.chromium.org/luci/common/testing/assertions"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCollectParse_NoArgs(t *testing.T) {
	Convey(`Make sure that Parse works with no arguments.`, t, func() {
		c := collectRun{}
		c.Init(auth.Options{})

		err := c.Parse(&[]string{})
		So(err, ShouldErrLike, "must provide -server")
	})
}

func TestCollectParse_NoInput(t *testing.T) {
	Convey(`Make sure that Parse handles no task IDs given.`, t, func() {
		c := collectRun{}
		c.Init(auth.Options{})

		err := c.GetFlags().Parse([]string{"-server", "http://localhost:9050"})

		err = c.Parse(&[]string{})
		So(err, ShouldErrLike, "must specify at least one")
	})
}

func TestCollectParse_BadTaskID(t *testing.T) {
	Convey(`Make sure that Parse handles a malformed task ID.`, t, func() {
		c := collectRun{}
		c.Init(auth.Options{})

		err := c.GetFlags().Parse([]string{"-server", "http://localhost:9050"})

		err = c.Parse(&[]string{"$$$$$"})
		So(err, ShouldErrLike, "task ID")
	})
}

func TestCollectParse_BadTimeout(t *testing.T) {
	Convey(`Make sure that Parse handles a negative timeout.`, t, func() {
		c := collectRun{}
		c.Init(auth.Options{})

		err := c.GetFlags().Parse([]string{
			"-server", "http://localhost:9050",
			"-timeout", "-30m",
		})

		err = c.Parse(&[]string{"x81n8xn1b684n"})
		So(err, ShouldErrLike, "negative timeout")
	})
}

func testCollectPollWithServer(runner *collectRun, s *testService) taskResult {
	c, clk := testclock.UseTime(context.Background(), testclock.TestRecentTimeLocal)
	c, _ = clock.WithTimeout(c, 100*time.Second)

	// Set a callback to make the timer finish.
	clk.SetTimerCallback(func(amt time.Duration, t clock.Timer) {
		clk.Add(amt)
	})

	results := make(chan taskResult, 1)
	go runner.pollForTaskResult(c, "10982374012938470", s, results)
	ret := <-results
	return ret
}

func TestCollectPollForTaskResult(t *testing.T) {
	t.Parallel()

	Convey(`Test fatal response`, t, func() {
		service := &testService{
			getTaskResult: func(c context.Context, _ string, _ bool) (*swarming.SwarmingRpcsTaskResult, error) {
				return nil, &googleapi.Error{Code: 404}
			},
		}
		result := testCollectPollWithServer(&collectRun{taskOutput: taskOutputNone}, service)
		So(result.err, ShouldErrLike, "404")
	})

	Convey(`Test timeout exceeded`, t, func() {
		service := &testService{
			getTaskResult: func(c context.Context, _ string, _ bool) (*swarming.SwarmingRpcsTaskResult, error) {
				return nil, &googleapi.Error{Code: 502}
			},
		}
		result := testCollectPollWithServer(&collectRun{taskOutput: taskOutputNone}, service)
		So(result.err, ShouldErrLike, "context deadline exceeded")
	})

	Convey(`Test bot finished`, t, func() {
		var written_to string
		var written_isolated string
		service := &testService{
			getTaskResult: func(c context.Context, _ string, _ bool) (*swarming.SwarmingRpcsTaskResult, error) {
				return &swarming.SwarmingRpcsTaskResult{
					State:      "COMPLETED",
					OutputsRef: &swarming.SwarmingRpcsFilesRef{Isolated: "aaaaaaaaa"},
				}, nil
			},
			getTaskOutput: func(c context.Context, _ string) (*swarming.SwarmingRpcsTaskOutput, error) {
				return &swarming.SwarmingRpcsTaskOutput{Output: "yipeeee"}, nil
			},
			getTaskOutputs: func(c context.Context, _, output string, ref *swarming.SwarmingRpcsFilesRef) ([]string, error) {
				written_to = output
				written_isolated = ref.Isolated
				return []string{"hello"}, nil
			},
		}
		runner := &collectRun{
			taskOutput: taskOutputAll,
			outputDir:  "bah",
		}
		result := testCollectPollWithServer(runner, service)
		So(result.err, ShouldBeNil)
		So(result.result, ShouldNotBeNil)
		So(result.result.State, ShouldResemble, "COMPLETED")
		So(result.output, ShouldResemble, "yipeeee")
		So(result.outputs, ShouldResemble, []string{"hello"})
		So(written_to, ShouldResemble, "bah")
		So(written_isolated, ShouldResemble, "aaaaaaaaa")
	})

	Convey(`Test bot finished after failures`, t, func() {
		i := 0
		maxTries := 5
		service := &testService{
			getTaskResult: func(c context.Context, _ string, _ bool) (*swarming.SwarmingRpcsTaskResult, error) {
				if i < maxTries {
					i += 1
					return nil, &googleapi.Error{Code: http.StatusInternalServerError}
				}
				return &swarming.SwarmingRpcsTaskResult{State: "COMPLETED"}, nil
			},
		}
		result := testCollectPollWithServer(&collectRun{taskOutput: taskOutputNone}, service)
		So(i, ShouldEqual, maxTries)
		So(result.err, ShouldBeNil)
		So(result.result, ShouldNotBeNil)
		So(result.result.State, ShouldResemble, "COMPLETED")
	})
}
