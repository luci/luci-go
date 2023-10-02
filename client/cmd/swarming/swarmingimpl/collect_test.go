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

package swarmingimpl

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"golang.org/x/sync/semaphore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"

	"go.chromium.org/luci/client/cmd/swarming/swarmingimpl/base"
	"go.chromium.org/luci/swarming/client/swarming"
	"go.chromium.org/luci/swarming/client/swarming/swarmingtest"
	swarmingv2 "go.chromium.org/luci/swarming/proto/api_v2"
)

func TestCollectParse(t *testing.T) {
	t.Parallel()

	expectErr := func(argv []string, errLike string) {
		_, _, code, _, stderr := SubcommandTest(
			context.Background(),
			CmdCollect,
			append([]string{"-server", "example.com"}, argv...),
			nil, nil,
		)
		So(code, ShouldEqual, 1)
		So(stderr, ShouldContainSubstring, errLike)
	}

	Convey(`Make sure that Parse handles no task IDs given.`, t, func() {
		expectErr(nil, "must specify at least one task id")
	})

	Convey(`Make sure that Parse handles a malformed task ID.`, t, func() {
		expectErr([]string{"$$$$$"}, "must be hex")
	})

	Convey(`Make sure that Parse handles a negative timeout.`, t, func() {
		expectErr([]string{"-timeout", "-30m", "aaaaaaaaa"}, "negative timeout")
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

func testCollectPollWithServer(runner *collectImpl, s swarming.Client) taskResult {
	ctx, cancel := setupClock()
	defer cancel()

	return runner.pollForTaskResult(ctx, "10982374012938470", s, semaphore.NewWeighted(1), &testAuthFlags{})
}

func testCollectPollForTasks(runner *collectImpl, taskIDs []string, s swarming.Client, downloadSem weightedSemaphore) []taskResult {
	ctx, cancel := setupClock()
	defer cancel()

	if downloadSem == nil {
		downloadSem = semaphore.NewWeighted(int64(len(taskIDs)))
	}

	return runner.pollForTasks(ctx, taskIDs, s, downloadSem, &testAuthFlags{})
}

func TestCollectPollForTaskResult(t *testing.T) {
	t.Parallel()

	Convey(`Test fatal response`, t, func() {
		service := &swarmingtest.Client{
			TaskResultMock: func(c context.Context, _ string, _ *swarming.TaskResultFields) (*swarmingv2.TaskResultResponse, error) {
				return nil, status.Errorf(codes.NotFound, "not found")
			},
		}
		result := testCollectPollWithServer(&collectImpl{taskOutput: taskOutputNone}, service)
		So(result.err, ShouldErrLike, "not found")
	})

	Convey(`Test bot finished with outputs on CAS`, t, func() {
		var writtenTo string
		var writtenInstance string
		var writtenDigest string
		service := &swarmingtest.Client{
			TaskResultMock: func(c context.Context, _ string, _ *swarming.TaskResultFields) (*swarmingv2.TaskResultResponse, error) {
				return &swarmingv2.TaskResultResponse{
					State: swarmingv2.TaskState_COMPLETED,
					CasOutputRoot: &swarmingv2.CASReference{
						CasInstance: "test-instance",
						Digest:      &swarmingv2.Digest{Hash: "aaaaaaaaa", SizeBytes: 111111},
					},
				}, nil
			},
			TaskOutputMock: func(c context.Context, _ string) (*swarmingv2.TaskOutputResponse, error) {
				return &swarmingv2.TaskOutputResponse{Output: []byte("yipeeee")}, nil
			},
			FilesFromCASMock: func(c context.Context, outdir string, casRef *swarmingv2.CASReference) ([]string, error) {
				writtenTo = outdir
				writtenInstance = casRef.CasInstance
				writtenDigest = fmt.Sprintf("%s/%d", casRef.Digest.Hash, casRef.Digest.SizeBytes)
				return []string{"hello"}, nil
			},
		}
		runner := &collectImpl{
			taskOutput: taskOutputAll,
			outputDir:  "bah",
		}
		result := testCollectPollWithServer(runner, service)
		So(result.err, ShouldBeNil)
		So(result.result, ShouldNotBeNil)
		So(result.result.State, ShouldEqual, swarmingv2.TaskState_COMPLETED)
		So(result.output, ShouldResemble, "yipeeee")
		So(result.outputs, ShouldResemble, []string{"hello"})
		So(writtenTo, ShouldStartWith, "bah")
		So(writtenInstance, ShouldResemble, "test-instance")
		So(writtenDigest, ShouldResemble, "aaaaaaaaa/111111")
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

	// TODO(vadimsh): Refactor to test through SubcommandTest to test command line
	// parsing.

	Convey(`Test eager return cancels polling goroutines`, t, func() {
		firstID, lastID := "1", "2"
		outputFetched := sync.Map{}

		service := &swarmingtest.Client{
			TaskResultMock: func(c context.Context, taskID string, _ *swarming.TaskResultFields) (*swarmingv2.TaskResultResponse, error) {
				if taskID != firstID {
					// Simulate the second task not finishing until the first
					// task has already finished, downloaded its outputs, and
					// canceled the context.
					<-c.Done()
					return nil, status.Errorf(codes.Canceled, "%s", c.Err())
				}
				return &swarmingv2.TaskResultResponse{
					State: swarmingv2.TaskState_COMPLETED,
					CasOutputRoot: &swarmingv2.CASReference{
						CasInstance: "test-instance",
						Digest:      &swarmingv2.Digest{Hash: "aaaaaaaaa", SizeBytes: 111111},
					},
				}, nil
			},
			TaskOutputMock: func(c context.Context, taskID string) (*swarmingv2.TaskOutputResponse, error) {
				outputFetched.Store(taskID, true)
				return &swarmingv2.TaskOutputResponse{Output: []byte("yipeeee")}, nil
			},
		}
		runner := &collectImpl{
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
		So(results[0].result.State, ShouldEqual, swarmingv2.TaskState_COMPLETED)
		So(results[1].err, ShouldErrLike, context.Canceled)
		So(results[1].result, ShouldBeNil)
	})

	Convey(`Test eager return lets downloading complete`, t, func() {
		firstID, lastID := "1", "2"
		outputFetched := sync.Map{}

		runner := &collectImpl{
			taskOutput: taskOutputAll,
			eager:      true,
		}
		firstTaskComplete := make(chan struct{})
		lastTaskDownloading := make(chan struct{})
		service := &swarmingtest.Client{
			TaskResultMock: func(c context.Context, taskID string, _ *swarming.TaskResultFields) (*swarmingv2.TaskResultResponse, error) {
				return &swarmingv2.TaskResultResponse{
					State: swarmingv2.TaskState_COMPLETED,
					CasOutputRoot: &swarmingv2.CASReference{
						CasInstance: "test-instance",
						Digest:      &swarmingv2.Digest{Hash: "aaaaaaaaa", SizeBytes: 111111},
					},
				}, nil
			},
			TaskOutputMock: func(c context.Context, taskID string) (*swarmingv2.TaskOutputResponse, error) {
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
				return &swarmingv2.TaskOutputResponse{Output: []byte("yipeeee")}, nil
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
		So(results[0].result.State, ShouldResemble, swarmingv2.TaskState_COMPLETED)
		So(results[0].err, ShouldBeNil)
		So(results[0].result, ShouldNotBeNil)
	})

	Convey(`Test eager return with one task`, t, func() {
		taskID := "1"
		outputFetched := false

		service := &swarmingtest.Client{
			TaskResultMock: func(c context.Context, taskID string, _ *swarming.TaskResultFields) (*swarmingv2.TaskResultResponse, error) {
				return &swarmingv2.TaskResultResponse{
					State: swarmingv2.TaskState_COMPLETED,
					CasOutputRoot: &swarmingv2.CASReference{
						CasInstance: "test-instance",
						Digest:      &swarmingv2.Digest{Hash: "aaaaaaaaa", SizeBytes: 111111},
					},
				}, nil
			},
			TaskOutputMock: func(c context.Context, taskID string) (*swarmingv2.TaskOutputResponse, error) {
				outputFetched = true
				return &swarmingv2.TaskOutputResponse{Output: []byte("yipeeee")}, nil
			},
		}
		runner := &collectImpl{
			taskOutput: taskOutputAll,
			eager:      true,
		}

		results := testCollectPollForTasks(runner, []string{taskID}, service, nil)
		So(results, ShouldHaveLength, 1)
		So(outputFetched, ShouldBeTrue)
		So(results[0].err, ShouldBeNil)
		So(results[0].result, ShouldNotBeNil)
		So(results[0].result.State, ShouldEqual, swarmingv2.TaskState_COMPLETED)
	})
}

func TestCollectSummarizeResults(t *testing.T) {
	t.Parallel()

	runner := &collectImpl{
		taskOutput: taskOutputAll,
	}

	Convey(`Generates json.`, t, func() {
		result1 := &swarmingv2.TaskResultResponse{
			CurrentTaskSlice: 0,
			Duration:         1,
			ExitCode:         0,
			State:            swarmingv2.TaskState_COMPLETED,
			PerformanceStats: &swarmingv2.PerformanceStats{
				BotOverhead: 0.1,
				CacheTrim:   &swarmingv2.OperationStats{Duration: 0.1},
				Cleanup:     &swarmingv2.OperationStats{Duration: 0.1},
				// Stats with 0 value also should be kept.
				IsolatedDownload: &swarmingv2.CASOperationStats{
					Duration:            0.1,
					InitialNumberItems:  0,
					InitialSize:         0,
					ItemsCold:           []byte(""),
					ItemsHot:            []byte("download_hot"),
					NumItemsCold:        0,
					NumItemsHot:         1,
					TotalBytesItemsCold: 0,
					TotalBytesItemsHot:  1,
				},
				IsolatedUpload: &swarmingv2.CASOperationStats{
					Duration:            0.1,
					InitialNumberItems:  0,
					InitialSize:         0,
					ItemsCold:           []byte(""),
					ItemsHot:            []byte("upload_hot"),
					NumItemsCold:        0,
					NumItemsHot:         1,
					TotalBytesItemsCold: 0,
					TotalBytesItemsHot:  1,
				},
				NamedCachesInstall:   &swarmingv2.OperationStats{Duration: 0.1},
				NamedCachesUninstall: &swarmingv2.OperationStats{Duration: 0.1},
				PackageInstallation:  &swarmingv2.OperationStats{Duration: 0.1},
			},
		}
		result2 := &swarmingv2.TaskResultResponse{
			CurrentTaskSlice: 1,
			Duration:         1,
			ExitCode:         -1,
			State:            swarmingv2.TaskState_COMPLETED,
			PerformanceStats: &swarmingv2.PerformanceStats{
				BotOverhead:          0.1,
				CacheTrim:            &swarmingv2.OperationStats{},
				Cleanup:              &swarmingv2.OperationStats{},
				IsolatedDownload:     &swarmingv2.CASOperationStats{},
				IsolatedUpload:       &swarmingv2.CASOperationStats{},
				NamedCachesInstall:   &swarmingv2.OperationStats{},
				NamedCachesUninstall: &swarmingv2.OperationStats{},
				PackageInstallation:  &swarmingv2.OperationStats{},
			},
		}
		result3 := &swarmingv2.TaskResultResponse{
			CurrentTaskSlice: 0,
			Duration:         1,
			ExitCode:         -1,
			State:            swarmingv2.TaskState_KILLED,
			PerformanceStats: &swarmingv2.PerformanceStats{
				BotOverhead: 0.1,
				CacheTrim:   &swarmingv2.OperationStats{Duration: 0.1},
				Cleanup:     &swarmingv2.OperationStats{Duration: 0.1},
				IsolatedDownload: &swarmingv2.CASOperationStats{
					Duration:            0.1,
					InitialNumberItems:  0,
					InitialSize:         0,
					ItemsCold:           []byte(""),
					ItemsHot:            []byte("download_hot"),
					NumItemsCold:        0,
					NumItemsHot:         1,
					TotalBytesItemsCold: 0,
					TotalBytesItemsHot:  1,
				},
				IsolatedUpload:       &swarmingv2.CASOperationStats{},
				NamedCachesInstall:   &swarmingv2.OperationStats{Duration: 0.1},
				NamedCachesUninstall: &swarmingv2.OperationStats{},
				PackageInstallation:  &swarmingv2.OperationStats{Duration: 0.1},
			},
		}
		result4 := &swarmingv2.TaskResultResponse{
			CurrentTaskSlice: 0,
			Duration:         1,
			State:            swarmingv2.TaskState_RUNNING,
		}
		results := []taskResult{
			{
				taskID: "task1",
				result: result1,
				output: "Output",
			},
			{
				taskID: "task2",
				result: result2,
				output: "Output",
			},
			{
				taskID: "task3",
				result: result3,
				output: "Output",
			},
			{
				taskID: "task4",
				result: result4,
				output: "Output",
			},
		}
		summary, err := runner.summarizeResults(results)
		So(err, ShouldBeNil)
		expected := `{
 "task1": {
  "output": "Output",
  "results": {
   "duration": 1,
   "state": "COMPLETED",
   "performance_stats": {
    "bot_overhead": 0.1,
    "isolated_download": {
     "duration": 0.1,
     "items_hot": "ZG93bmxvYWRfaG90",
     "num_items_hot": "1",
     "total_bytes_items_hot": "1"
    },
    "isolated_upload": {
     "duration": 0.1,
     "items_hot": "dXBsb2FkX2hvdA==",
     "num_items_hot": "1",
     "total_bytes_items_hot": "1"
    },
    "package_installation": {
     "duration": 0.1
    },
    "cache_trim": {
     "duration": 0.1
    },
    "named_caches_install": {
     "duration": 0.1
    },
    "named_caches_uninstall": {
     "duration": 0.1
    },
    "cleanup": {
     "duration": 0.1
    }
   }
  }
 },
 "task2": {
  "output": "Output",
  "results": {
   "duration": 1,
   "exit_code": "-1",
   "state": "COMPLETED",
   "performance_stats": {
    "bot_overhead": 0.1,
    "isolated_download": {},
    "isolated_upload": {},
    "package_installation": {},
    "cache_trim": {},
    "named_caches_install": {},
    "named_caches_uninstall": {},
    "cleanup": {}
   },
   "current_task_slice": 1
  }
 },
 "task3": {
  "output": "Output",
  "results": {
   "duration": 1,
   "exit_code": "-1",
   "state": "KILLED",
   "performance_stats": {
    "bot_overhead": 0.1,
    "isolated_download": {
     "duration": 0.1,
     "items_hot": "ZG93bmxvYWRfaG90",
     "num_items_hot": "1",
     "total_bytes_items_hot": "1"
    },
    "isolated_upload": {},
    "package_installation": {
     "duration": 0.1
    },
    "cache_trim": {
     "duration": 0.1
    },
    "named_caches_install": {
     "duration": 0.1
    },
    "named_caches_uninstall": {},
    "cleanup": {
     "duration": 0.1
    }
   }
  }
 },
 "task4": {
  "output": "Output",
  "results": {
   "duration": 1,
   "state": "RUNNING"
  }
 }
}`
		actual, _ := base.EncodeJSON(summary)
		So(string(actual), ShouldEqual, expected)
	})
}

func TestCollectSummarizeResultsPython(t *testing.T) {
	t.Parallel()

	Convey(`Simple json.`, t, func() {
		result := &swarmingv2.TaskResultResponse{
			State:    swarmingv2.TaskState_COMPLETED,
			Duration: 1,
			ExitCode: 0,
		}
		results := []taskResult{
			{
				result: result,
				output: "Output",
			},
			{},
		}
		summary, err := summarizeResultsPython(results)
		So(err, ShouldBeNil)
		actual, _ := base.EncodeJSON(summary)
		So(string(actual), ShouldEqual, `{
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
