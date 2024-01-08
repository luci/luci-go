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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/client/cmd/swarming/swarmingimpl/base"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
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

	Convey(`Make sure that Parse handles a dup task ID.`, t, func() {
		expectErr([]string{"aaaaaaaaa", "aaaaaaaaa"}, "given more than once")
	})

	Convey(`Make sure that Parse handles a negative timeout.`, t, func() {
		expectErr([]string{"-timeout", "-30m", "aaaaaaaaa"}, "negative timeout")
	})
}

func TestCollect(t *testing.T) {
	t.Parallel()

	casOutputDir := t.TempDir()

	Convey(`With mocks`, t, func() {
		sleeps := 0
		onSleep := func() {}

		ctx, clk := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
		clk.SetTimerCallback(func(d time.Duration, t clock.Timer) {
			clk.Add(d)
			sleeps += 1
			onSleep()
		})

		mockedState := map[string]swarmingv2.TaskState{}
		mockedHasCAS := map[string]bool{}
		mockedErr := map[string]codes.Code{}
		mockedStdoutErr := error(nil)
		mockedCASErr := error(nil)

		service := &swarmingtest.Client{
			TaskResultsMock: func(ctx context.Context, taskIDs []string, fields *swarming.TaskResultFields) ([]swarming.ResultOrErr, error) {
				out := make([]swarming.ResultOrErr, len(taskIDs))
				for i, taskID := range taskIDs {
					if code, ok := mockedErr[taskID]; ok {
						out[i] = swarming.ResultOrErr{Err: status.Errorf(code, "some error")}
					} else if state, ok := mockedState[taskID]; ok {
						out[i] = swarming.ResultOrErr{
							Result: &swarmingv2.TaskResultResponse{
								TaskId: taskID,
								State:  state,
							},
						}
						if mockedHasCAS[taskID] {
							out[i].Result.CasOutputRoot = &swarmingv2.CASReference{
								CasInstance: "cas-instance",
								Digest: &swarmingv2.Digest{
									Hash: "cas-" + taskID,
								},
							}
						}
					} else {
						panic(fmt.Sprintf("unexpected task %q", taskID))
					}
				}
				return out, nil
			},

			TaskOutputMock: func(ctx context.Context, taskID string) (*swarmingv2.TaskOutputResponse, error) {
				return &swarmingv2.TaskOutputResponse{
					Output: []byte(fmt.Sprintf("Output of %s", taskID)),
				}, mockedStdoutErr
			},

			FilesFromCASMock: func(ctx context.Context, outdir string, casRef *swarmingv2.CASReference) ([]string, error) {
				if casRef.CasInstance != "cas-instance" {
					panic("unexpected CAS instance")
				}
				if !strings.HasPrefix(casRef.Digest.Hash, "cas-") {
					panic("unexpected fake digest")
				}
				taskID := casRef.Digest.Hash[len("cas-"):]
				if want := filepath.Join(casOutputDir, taskID); want != outdir {
					panic(fmt.Sprintf("expecting out dir %q, got %q", want, outdir))
				}
				return []string{"out-" + taskID}, mockedCASErr
			},
		}

		Convey(`Happy path`, func() {
			mockedState["a0"] = swarmingv2.TaskState_PENDING
			mockedHasCAS["a0"] = true

			mockedState["a1"] = swarmingv2.TaskState_COMPLETED
			mockedHasCAS["a1"] = true

			mockedState["a2"] = swarmingv2.TaskState_COMPLETED
			mockedHasCAS["a2"] = false

			onSleep = func() {
				mockedState["a0"] = swarmingv2.TaskState_COMPLETED
			}

			res, _, code, _, _ := SubcommandTest(
				ctx,
				CmdCollect,
				[]string{
					"-server", "example.com",
					"-json-output", "-",
					"-task-output-stdout", "json",
					"-output-dir", casOutputDir,
					"a1", "a0", "a2",
				},
				nil, service,
			)
			So(code, ShouldEqual, 0)

			json, _ := base.EncodeJSON(res)
			So(string(json), ShouldEqual, `{
 "a0": {
  "output": "Output of a0",
  "outputs": [
   "out-a0"
  ],
  "results": {
   "task_id": "a0",
   "cas_output_root": {
    "cas_instance": "cas-instance",
    "digest": {
     "hash": "cas-a0"
    }
   },
   "state": "COMPLETED"
  }
 },
 "a1": {
  "output": "Output of a1",
  "outputs": [
   "out-a1"
  ],
  "results": {
   "task_id": "a1",
   "cas_output_root": {
    "cas_instance": "cas-instance",
    "digest": {
     "hash": "cas-a1"
    }
   },
   "state": "COMPLETED"
  }
 },
 "a2": {
  "output": "Output of a2",
  "results": {
   "task_id": "a2",
   "state": "COMPLETED"
  }
 }
}`)

			// Check actually created output directories, even for tasks with no
			// outputs.
			for _, taskID := range []string{"a0", "a1", "a2"} {
				s, err := os.Stat(filepath.Join(casOutputDir, taskID))
				So(err, ShouldBeNil)
				So(s.IsDir(), ShouldBeTrue)
			}
		})

		Convey(`Collect error`, func() {
			mockedState["a0"] = swarmingv2.TaskState_COMPLETED
			mockedErr["a1"] = codes.PermissionDenied

			res, _, code, _, _ := SubcommandTest(
				ctx,
				CmdCollect,
				[]string{
					"-server", "example.com",
					"-json-output", "-",
					"-task-output-stdout", "json",
					"-output-dir", casOutputDir,
					"a0", "a1",
				},
				nil, service,
			)
			So(code, ShouldEqual, 0)

			json, _ := base.EncodeJSON(res)
			So(string(json), ShouldEqual, `{
 "a0": {
  "output": "Output of a0",
  "results": {
   "task_id": "a0",
   "state": "COMPLETED"
  }
 },
 "a1": {
  "error": "rpc error: code = PermissionDenied desc = some error"
 }
}`)
		})

		Convey(`Stdout fetch error`, func() {
			mockedState["a0"] = swarmingv2.TaskState_COMPLETED
			mockedStdoutErr = errors.New("boom")

			res, _, code, _, _ := SubcommandTest(
				ctx,
				CmdCollect,
				[]string{
					"-server", "example.com",
					"-json-output", "-",
					"-task-output-stdout", "json",
					"-output-dir", casOutputDir,
					"a0",
				},
				nil, service,
			)
			So(code, ShouldEqual, 0)

			json, _ := base.EncodeJSON(res)
			So(string(json), ShouldEqual, `{
 "a0": {
  "error": "fetching console output of a0: boom",
  "results": {
   "task_id": "a0",
   "state": "COMPLETED"
  }
 }
}`)
		})

		Convey(`CAS fetch error`, func() {
			mockedState["a0"] = swarmingv2.TaskState_COMPLETED
			mockedHasCAS["a0"] = true
			mockedCASErr = errors.New("boom")

			res, _, code, _, _ := SubcommandTest(
				ctx,
				CmdCollect,
				[]string{
					"-server", "example.com",
					"-json-output", "-",
					"-task-output-stdout", "json",
					"-output-dir", casOutputDir,
					"a0",
				},
				nil, service,
			)
			So(code, ShouldEqual, 0)

			json, _ := base.EncodeJSON(res)
			So(string(json), ShouldEqual, `{
 "a0": {
  "error": "fetching isolated output of a0: boom",
  "output": "Output of a0",
  "results": {
   "task_id": "a0",
   "cas_output_root": {
    "cas_instance": "cas-instance",
    "digest": {
     "hash": "cas-a0"
    }
   },
   "state": "COMPLETED"
  }
 }
}`)
		})

		Convey(`Timeout waiting`, func() {
			mockedState["a0"] = swarmingv2.TaskState_PENDING

			res, _, code, _, _ := SubcommandTest(
				ctx,
				CmdCollect,
				[]string{
					"-server", "example.com",
					"-json-output", "-",
					"-task-output-stdout", "json",
					"-output-dir", casOutputDir,
					"-timeout", "1h",
					"a0",
				},
				nil, service,
			)
			So(code, ShouldEqual, 0)

			json, _ := base.EncodeJSON(res)
			So(string(json), ShouldEqual, `{
 "a0": {
  "error": "rpc_timeout",
  "results": {
   "task_id": "a0",
   "state": "PENDING"
  }
 }
}`)
		})

		Convey(`No waiting`, func() {
			mockedState["a0"] = swarmingv2.TaskState_PENDING
			mockedState["a1"] = swarmingv2.TaskState_PENDING

			onSleep = func() {
				panic("must not sleep")
			}

			res, _, code, _, _ := SubcommandTest(
				ctx,
				CmdCollect,
				[]string{
					"-server", "example.com",
					"-json-output", "-",
					"-task-output-stdout", "json",
					"-output-dir", casOutputDir,
					"-wait=false",
					"a1", "a0",
				},
				nil, service,
			)
			So(code, ShouldEqual, 0)

			json, _ := base.EncodeJSON(res)
			So(string(json), ShouldEqual, `{
 "a0": {
  "results": {
   "task_id": "a0",
   "state": "PENDING"
  }
 },
 "a1": {
  "results": {
   "task_id": "a1",
   "state": "PENDING"
  }
 }
}`)
		})

		Convey(`Waiting any`, func() {
			mockedState["a0"] = swarmingv2.TaskState_PENDING
			mockedState["a1"] = swarmingv2.TaskState_PENDING
			mockedState["a2"] = swarmingv2.TaskState_PENDING

			onSleep = func() {
				mockedState["a1"] = swarmingv2.TaskState_COMPLETED
				mockedState["a2"] = swarmingv2.TaskState_COMPLETED
			}

			res, _, code, _, _ := SubcommandTest(
				ctx,
				CmdCollect,
				[]string{
					"-server", "example.com",
					"-json-output", "-",
					"-task-output-stdout", "json",
					"-output-dir", casOutputDir,
					"-eager",
					"a1", "a0", "a2",
				},
				nil, service,
			)
			So(code, ShouldEqual, 0)

			json, _ := base.EncodeJSON(res)
			So(string(json), ShouldEqual, `{
 "a0": {
  "results": {
   "task_id": "a0",
   "state": "PENDING"
  }
 },
 "a1": {
  "output": "Output of a1",
  "results": {
   "task_id": "a1",
   "state": "COMPLETED"
  }
 },
 "a2": {
  "output": "Output of a2",
  "results": {
   "task_id": "a2",
   "state": "COMPLETED"
  }
 }
}`)
		})
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

		results := map[string]taskResult{
			"task1": {
				taskID: "task1",
				result: result1,
				output: "Output",
			},
			"task2": {
				taskID: "task2",
				result: result2,
				output: "Output",
			},
			"task3": {
				taskID: "task3",
				result: result3,
				output: "Output",
			},
			"task4": {
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
		taskIDs := []string{"failed1", "finished", "failed2"}

		results := map[string]taskResult{
			"failed1": {
				err: errors.New("boom"),
			},
			"finished": {
				result: &swarmingv2.TaskResultResponse{
					State:    swarmingv2.TaskState_COMPLETED,
					Duration: 1,
					ExitCode: 0,
				},
				output: "Output",
			},
			"failed2": {
				err: errors.New("boom"),
			},
		}

		summary, err := summarizeResultsPython(taskIDs, results)
		So(err, ShouldBeNil)

		actual, _ := base.EncodeJSON(summary)
		So(string(actual), ShouldEqual, `{
 "shards": [
  null,
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
