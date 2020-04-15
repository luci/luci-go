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
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	googleapi "google.golang.org/api/googleapi"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/api/swarming/swarming/v1"
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

func setupClock() (context.Context, context.CancelFunc) {
	ctx, clk := testclock.UseTime(context.Background(), testclock.TestRecentTimeLocal)
	ctx, cancel := clock.WithTimeout(ctx, 100*time.Second)

	// Set a callback to make the timer finish.
	clk.SetTimerCallback(func(amt time.Duration, t clock.Timer) {
		clk.Add(amt)
	})
	return ctx, cancel
}

func testCollectPollWithServer(runner *collectRun, s *testService) taskResult {
	ctx, cancel := setupClock()
	defer cancel()

	return runner.pollForTaskResult(ctx, "10982374012938470", s)
}

func testCollectPollForTasks(runner *collectRun, taskIDs []string, s *testService) []taskResult {
	ctx, cancel := setupClock()
	defer cancel()

	return runner.pollForTasks(ctx, taskIDs, s)
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
		So(result.err, ShouldErrLike, "502")
	})

	Convey(`Test bot finished`, t, func() {
		var writtenTo string
		var writtenIsolated string
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
				writtenTo = output
				writtenIsolated = ref.Isolated
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
		So(writtenTo, ShouldResemble, "bah")
		So(writtenIsolated, ShouldResemble, "aaaaaaaaa")
	})

	Convey(`Test bot finished after failures`, t, func() {
		i := 0
		maxTries := 5
		service := &testService{
			getTaskResult: func(c context.Context, _ string, _ bool) (*swarming.SwarmingRpcsTaskResult, error) {
				if i < maxTries {
					i++
					return nil, &googleapi.Error{Code: http.StatusBadGateway}
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

func TestCollectPollForTasks(t *testing.T) {
	t.Parallel()

	Convey(`Test eager return cancels polling goroutines`, t, func() {
		firstID, lastID := "1", "2"
		outputFetched := make(map[string]bool)

		service := &testService{
			getTaskResult: func(c context.Context, taskID string, _ bool) (*swarming.SwarmingRpcsTaskResult, error) {
				if taskID != firstID {
					<-c.Done()
					return nil, c.Err()
				}
				return &swarming.SwarmingRpcsTaskResult{
					State:      "COMPLETED",
					OutputsRef: &swarming.SwarmingRpcsFilesRef{Isolated: "aaaaaaaaa"},
				}, nil
			},
			getTaskOutput: func(c context.Context, taskID string) (*swarming.SwarmingRpcsTaskOutput, error) {
				outputFetched[taskID] = true
				return &swarming.SwarmingRpcsTaskOutput{Output: "yipeeee"}, nil
			},
		}
		runner := &collectRun{
			taskOutput:        taskOutputAll,
			eager:             true,
			eagerDownloadWG:   &sync.WaitGroup{},
			firstTaskComplete: make(chan struct{}),
		}

		results := testCollectPollForTasks(runner, []string{firstID, lastID}, service)
		So(outputFetched[firstID], ShouldBeTrue)
		So(outputFetched[lastID], ShouldBeFalse)
		So(results[0].err, ShouldBeNil)
		So(results[0].result, ShouldNotBeNil)
		So(results[0].result.State, ShouldResemble, "COMPLETED")
		So(results[1].err, ShouldUnwrapTo, context.Canceled)
		So(results[1].result, ShouldBeNil)
	})

	Convey(`Test eager return lets downloading complete`, t, func() {
		firstID, lastID := "1", "2"
		outputFetched := make(map[string]bool)

		runner := &collectRun{
			taskOutput:        taskOutputAll,
			eager:             true,
			eagerDownloadWG:   &sync.WaitGroup{},
			firstTaskComplete: make(chan struct{}),
		}
		var contextCanceled error
		service := &testService{
			getTaskResult: func(c context.Context, taskID string, _ bool) (*swarming.SwarmingRpcsTaskResult, error) {
				return &swarming.SwarmingRpcsTaskResult{
					State:      "COMPLETED",
					OutputsRef: &swarming.SwarmingRpcsFilesRef{Isolated: "aaaaaaaaa"},
				}, nil
			},
			getTaskOutput: func(c context.Context, taskID string) (*swarming.SwarmingRpcsTaskOutput, error) {
				if taskID != firstID {
					<-runner.firstTaskComplete
					contextCanceled = c.Err()
				}
				outputFetched[taskID] = true
				return &swarming.SwarmingRpcsTaskOutput{Output: "yipeeee"}, nil
			},
		}

		results := testCollectPollForTasks(runner, []string{firstID, lastID}, service)
		So(contextCanceled, ShouldBeNil)
		So(outputFetched[firstID], ShouldBeTrue)
		So(outputFetched[lastID], ShouldBeTrue)
		So(results[0].err, ShouldBeNil)
		So(results[0].result, ShouldNotBeNil)
		So(results[0].result.State, ShouldResemble, "COMPLETED")
		So(results[0].err, ShouldBeNil)
		So(results[0].result, ShouldNotBeNil)
		So(results[0].result.State, ShouldResemble, "COMPLETED")
	})
}

func TestCollectSummarizeResultsPython(t *testing.T) {
	t.Parallel()

	Convey(`Simple json.`, t, func() {
		results := []taskResult{
			{
				result: &swarming.SwarmingRpcsTaskResult{
					State:    "COMPLETED",
					Duration: 1,
					ExitCode: 0,
				},
				output: "Output",
			},
			{},
		}
		json, err := summarizeResultsPython(results)
		So(err, ShouldBeNil)
		So(string(json), ShouldEqual, `{
  "shards": [
    {
      "duration": 1,
      "output": "Output",
      "state": "COMPLETED"
    },
    null
  ]
}`)
	})
}
