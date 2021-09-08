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

package lib

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	rbeclient "github.com/bazelbuild/remote-apis-sdks/go/pkg/client"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/sync/semaphore"
	googleapi "google.golang.org/api/googleapi"

	"go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestCollectParse_NoArgs(t *testing.T) {
	Convey(`Make sure that Parse works with no arguments.`, t, func() {
		c := collectRun{}
		c.Init(&testAuthFlags{})

		err := c.Parse(&[]string{})
		So(err, ShouldErrLike, "must provide -server")
	})
}

func TestCollectParse_NoInput(t *testing.T) {
	Convey(`Make sure that Parse handles no task IDs given.`, t, func() {
		c := collectRun{}
		c.Init(&testAuthFlags{})

		err := c.GetFlags().Parse([]string{"-server", "http://localhost:9050"})
		So(err, ShouldBeNil)

		err = c.Parse(&[]string{})
		So(err, ShouldErrLike, "must specify at least one")
	})
}

func TestCollectParse_BadTaskID(t *testing.T) {
	Convey(`Make sure that Parse handles a malformed task ID.`, t, func() {
		c := collectRun{}
		c.Init(&testAuthFlags{})

		err := c.GetFlags().Parse([]string{"-server", "http://localhost:9050"})
		So(err, ShouldBeNil)

		err = c.Parse(&[]string{"$$$$$"})
		So(err, ShouldErrLike, "task ID")
	})
}

func TestCollectParse_BadTimeout(t *testing.T) {
	Convey(`Make sure that Parse handles a negative timeout.`, t, func() {
		c := collectRun{}
		c.Init(&testAuthFlags{})

		err := c.GetFlags().Parse([]string{
			"-server", "http://localhost:9050",
			"-timeout", "-30m",
		})
		So(err, ShouldBeNil)

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

	return runner.pollForTaskResult(ctx, "10982374012938470", s, semaphore.NewWeighted(1))
}

func testCollectPollForTasks(runner *collectRun, taskIDs []string, s *testService, downloadSem weightedSemaphore) []taskResult {
	ctx, cancel := setupClock()
	defer cancel()

	if downloadSem == nil {
		downloadSem = semaphore.NewWeighted(int64(len(taskIDs)))
	}

	return runner.pollForTasks(ctx, taskIDs, s, downloadSem)
}

func TestCollectPollForTaskResult(t *testing.T) {
	t.Parallel()

	Convey(`Test fatal response`, t, func() {
		service := &testService{
			taskResult: func(c context.Context, _ string, _ bool) (*swarming.SwarmingRpcsTaskResult, error) {
				return nil, &googleapi.Error{Code: 404}
			},
		}
		result := testCollectPollWithServer(&collectRun{taskOutput: taskOutputNone}, service)
		So(result.err, ShouldErrLike, "404")
	})

	Convey(`Test timeout exceeded`, t, func() {
		service := &testService{
			taskResult: func(c context.Context, _ string, _ bool) (*swarming.SwarmingRpcsTaskResult, error) {
				return nil, &googleapi.Error{Code: 502}
			},
		}
		result := testCollectPollWithServer(&collectRun{taskOutput: taskOutputNone}, service)
		So(result.err, ShouldErrLike, "502")
	})

	Convey(`Test bot finished with outputs on Isolate server`, t, func() {
		var writtenTo string
		var writtenIsolated string
		service := &testService{
			taskResult: func(c context.Context, _ string, _ bool) (*swarming.SwarmingRpcsTaskResult, error) {
				return &swarming.SwarmingRpcsTaskResult{
					State:      "COMPLETED",
					OutputsRef: &swarming.SwarmingRpcsFilesRef{Isolated: "aaaaaaaaa"},
				}, nil
			},
			taskOutput: func(c context.Context, _ string) (*swarming.SwarmingRpcsTaskOutput, error) {
				return &swarming.SwarmingRpcsTaskOutput{Output: "yipeeee"}, nil
			},
			filesFromIsolate: func(c context.Context, output string, ref *swarming.SwarmingRpcsFilesRef) ([]string, error) {
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
		So(writtenTo, ShouldStartWith, "bah")
		So(writtenIsolated, ShouldResemble, "aaaaaaaaa")
	})

	Convey(`Test bot finished with outputs on CAS`, t, func() {
		var writtenTo string
		var writtenInstance string
		var writtenDigest string
		service := &testService{
			taskResult: func(c context.Context, _ string, _ bool) (*swarming.SwarmingRpcsTaskResult, error) {
				return &swarming.SwarmingRpcsTaskResult{
					State: "COMPLETED",
					CasOutputRoot: &swarming.SwarmingRpcsCASReference{
						CasInstance: "test-instance",
						Digest:      &swarming.SwarmingRpcsDigest{Hash: "aaaaaaaaa", SizeBytes: 111111},
					},
				}, nil
			},
			taskOutput: func(c context.Context, _ string) (*swarming.SwarmingRpcsTaskOutput, error) {
				return &swarming.SwarmingRpcsTaskOutput{Output: "yipeeee"}, nil
			},
			filesFromCAS: func(c context.Context, outdir string, _ *rbeclient.Client, casRef *swarming.SwarmingRpcsCASReference) ([]string, error) {
				writtenTo = outdir
				writtenInstance = casRef.CasInstance
				writtenDigest = fmt.Sprintf("%s/%d", casRef.Digest.Hash, casRef.Digest.SizeBytes)
				return []string{"hello"}, nil
			},
		}
		runner := &collectRun{
			taskOutput: taskOutputAll,
			outputDir:  "bah",
		}
		runner.authFlags = &testAuthFlags{}
		result := testCollectPollWithServer(runner, service)
		So(result.err, ShouldBeNil)
		So(result.result, ShouldNotBeNil)
		So(result.result.State, ShouldResemble, "COMPLETED")
		So(result.output, ShouldResemble, "yipeeee")
		So(result.outputs, ShouldResemble, []string{"hello"})
		So(writtenTo, ShouldStartWith, "bah")
		So(writtenInstance, ShouldResemble, "test-instance")
		So(writtenDigest, ShouldResemble, "aaaaaaaaa/111111")
	})

	Convey(`Test bot finished after failures`, t, func() {
		i := 0
		maxTries := 5
		service := &testService{
			taskResult: func(c context.Context, _ string, _ bool) (*swarming.SwarmingRpcsTaskResult, error) {
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

// mockSemaphore is a thin wrapper around semaphore.Weighted that adds a
// notification hook to Acquire().
type mockSemaphore struct {
	*semaphore.Weighted

	acquireCalls chan int64
}

func newMockSemaphore(n int64) *mockSemaphore {
	return &mockSemaphore{
		Weighted:     semaphore.NewWeighted(n),
		acquireCalls: make(chan int64),
	}
}

func (s *mockSemaphore) Acquire(ctx context.Context, n int64) error {
	s.acquireCalls <- n
	return s.Weighted.Acquire(ctx, n)
}

func TestCollectPollForTasks(t *testing.T) {
	t.Parallel()

	Convey(`Test eager return cancels polling goroutines`, t, func() {
		firstID, lastID := "1", "2"
		outputFetched := sync.Map{}

		service := &testService{
			taskResult: func(c context.Context, taskID string, _ bool) (*swarming.SwarmingRpcsTaskResult, error) {
				if taskID != firstID {
					// Simulate the second task not finishing until the first
					// task has already finished, downloaded its outputs, and
					// canceled the context.
					<-c.Done()
					return nil, c.Err()
				}
				return &swarming.SwarmingRpcsTaskResult{
					State:      "COMPLETED",
					OutputsRef: &swarming.SwarmingRpcsFilesRef{Isolated: "aaaaaaaaa"},
				}, nil
			},
			taskOutput: func(c context.Context, taskID string) (*swarming.SwarmingRpcsTaskOutput, error) {
				outputFetched.Store(taskID, true)
				return &swarming.SwarmingRpcsTaskOutput{Output: "yipeeee"}, nil
			},
		}
		runner := &collectRun{
			taskOutput: taskOutputAll,
			eager:      true,
		}

		taskIDs := []string{firstID, lastID}
		results := testCollectPollForTasks(runner, taskIDs, service, nil)
		So(results, ShouldHaveLength, len(taskIDs))
		_, firstOutputFetched := outputFetched.Load(firstID)
		_, lastOutputFetched := outputFetched.Load(lastID)
		So(firstOutputFetched, ShouldBeTrue)
		So(lastOutputFetched, ShouldBeFalse)
		So(results[0].err, ShouldBeNil)
		So(results[0].result, ShouldNotBeNil)
		So(results[0].result.State, ShouldResemble, "COMPLETED")
		So(results[1].err, ShouldUnwrapTo, context.Canceled)
		So(results[1].result, ShouldBeNil)
	})

	Convey(`Test eager return lets downloading complete`, t, func() {
		firstID, lastID := "1", "2"
		outputFetched := sync.Map{}

		runner := &collectRun{
			taskOutput: taskOutputAll,
			eager:      true,
		}
		firstTaskComplete := make(chan struct{})
		lastTaskDownloading := make(chan struct{})
		service := &testService{
			taskResult: func(c context.Context, taskID string, _ bool) (*swarming.SwarmingRpcsTaskResult, error) {
				return &swarming.SwarmingRpcsTaskResult{
					State:      "COMPLETED",
					OutputsRef: &swarming.SwarmingRpcsFilesRef{Isolated: "aaaaaaaaa"},
				}, nil
			},
			taskOutput: func(c context.Context, taskID string) (*swarming.SwarmingRpcsTaskOutput, error) {
				// Make sure that the two tasks are downloading outputs at the
				// same time, but have the second task wait for the first task's
				// download to complete before continuing the download.
				if taskID == firstID {
					<-lastTaskDownloading
				} else {
					close(lastTaskDownloading)
					<-firstTaskComplete
				}
				outputFetched.Store(taskID, true)
				return &swarming.SwarmingRpcsTaskOutput{Output: "yipeeee"}, nil
			},
		}

		taskIDs := []string{firstID, lastID}
		downloadSem := newMockSemaphore(int64(len(taskIDs)))
		defer close(downloadSem.acquireCalls)
		go func() {
			// When the semaphore is acquired with an argument equal to the
			// number of tasks, that means the first task has finished
			// downloading and is triggering the eager return mechanism.
			for n := range downloadSem.acquireCalls {
				if n == int64(len(taskIDs)) {
					close(firstTaskComplete)
				}
			}
		}()

		results := testCollectPollForTasks(runner, taskIDs, service, downloadSem)
		So(results, ShouldHaveLength, len(taskIDs))
		_, firstOutputFetched := outputFetched.Load(firstID)
		_, lastOutputFetched := outputFetched.Load(lastID)
		So(firstOutputFetched, ShouldBeTrue)
		So(lastOutputFetched, ShouldBeTrue)
		So(results[0].err, ShouldBeNil)
		So(results[0].result, ShouldNotBeNil)
		So(results[0].result.State, ShouldResemble, "COMPLETED")
		So(results[0].err, ShouldBeNil)
		So(results[0].result, ShouldNotBeNil)
		So(results[0].result.State, ShouldResemble, "COMPLETED")
	})

	Convey(`Test eager return with one task`, t, func() {
		taskID := "1"
		outputFetched := false

		service := &testService{
			taskResult: func(c context.Context, taskID string, _ bool) (*swarming.SwarmingRpcsTaskResult, error) {
				return &swarming.SwarmingRpcsTaskResult{
					State:      "COMPLETED",
					OutputsRef: &swarming.SwarmingRpcsFilesRef{Isolated: "aaaaaaaaa"},
				}, nil
			},
			taskOutput: func(c context.Context, taskID string) (*swarming.SwarmingRpcsTaskOutput, error) {
				outputFetched = true
				return &swarming.SwarmingRpcsTaskOutput{Output: "yipeeee"}, nil
			},
		}
		runner := &collectRun{
			taskOutput: taskOutputAll,
			eager:      true,
		}

		results := testCollectPollForTasks(runner, []string{taskID}, service, nil)
		So(results, ShouldHaveLength, 1)
		So(outputFetched, ShouldBeTrue)
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
