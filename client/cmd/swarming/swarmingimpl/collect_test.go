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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/client/cmd/swarming/swarmingimpl/output"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/swarming/client/swarming"
	"go.chromium.org/luci/swarming/client/swarming/swarmingtest"
	swarmingv2 "go.chromium.org/luci/swarming/proto/api_v2"
)

func TestCollectParse(t *testing.T) {
	t.Parallel()

	expectErr := func(argv []string, errLike string) {
		_, code, _, stderr := SubcommandTest(
			context.Background(),
			CmdCollect,
			append([]string{"-server", "example.com"}, argv...),
			nil, nil,
		)
		assert.Loosely(t, code, should.Equal(1))
		assert.Loosely(t, stderr, should.ContainSubstring(errLike))
	}

	ftt.Run(`Make sure that Parse handles no task IDs given.`, t, func(t *ftt.Test) {
		expectErr(nil, "must specify at least one task id")
	})

	ftt.Run(`Make sure that Parse handles a malformed task ID.`, t, func(t *ftt.Test) {
		expectErr([]string{"$$$$$"}, "must be hex")
	})

	ftt.Run(`Make sure that Parse handles a dup task ID.`, t, func(t *ftt.Test) {
		expectErr([]string{"aaaaaaaaa", "aaaaaaaaa"}, "given more than once")
	})

	ftt.Run(`Make sure that Parse handles a negative timeout.`, t, func(t *ftt.Test) {
		expectErr([]string{"-timeout", "-30m", "aaaaaaaaa"}, "negative timeout")
	})
}

func TestCollect(t *testing.T) {
	t.Parallel()

	casOutputDir := t.TempDir()

	ftt.Run(`With mocks`, t, func(t *ftt.Test) {
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

			TaskOutputMock: func(ctx context.Context, taskID string, out io.Writer) (swarmingv2.TaskState, error) {
				if mockedStdoutErr != nil {
					return 0, mockedStdoutErr
				}
				_, err := fmt.Fprintf(out, "Output of %s", taskID)
				return swarmingv2.TaskState_COMPLETED, err
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

		t.Run(`Happy path`, func(t *ftt.Test) {
			mockedState["a0"] = swarmingv2.TaskState_PENDING
			mockedHasCAS["a0"] = true

			mockedState["a1"] = swarmingv2.TaskState_COMPLETED
			mockedHasCAS["a1"] = true

			mockedState["a2"] = swarmingv2.TaskState_COMPLETED
			mockedHasCAS["a2"] = false

			onSleep = func() {
				mockedState["a0"] = swarmingv2.TaskState_COMPLETED
			}

			_, code, stdout, _ := SubcommandTest(
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
			assert.Loosely(t, code, should.BeZero)
			assert.Loosely(t, sortJSON(stdout), should.Equal(`{
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
}
`))

			// Check actually created output directories, even for tasks with no
			// outputs.
			for _, taskID := range []string{"a0", "a1", "a2"} {
				s, err := os.Stat(filepath.Join(casOutputDir, taskID))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, s.IsDir(), should.BeTrue)
			}
		})

		t.Run(`Collect error`, func(t *ftt.Test) {
			mockedState["a0"] = swarmingv2.TaskState_COMPLETED
			mockedErr["a1"] = codes.PermissionDenied

			_, code, stdout, _ := SubcommandTest(
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
			assert.Loosely(t, code, should.BeZero)
			assert.Loosely(t, sortJSON(stdout), should.Equal(`{
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
}
`))
		})

		t.Run(`Stdout fetch error`, func(t *ftt.Test) {
			mockedState["a0"] = swarmingv2.TaskState_COMPLETED
			mockedStdoutErr = errors.New("boom")

			_, code, stdout, _ := SubcommandTest(
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
			assert.Loosely(t, code, should.BeZero)
			assert.Loosely(t, sortJSON(stdout), should.Equal(`{
 "a0": {
  "error": "fetching console output of a0: boom",
  "results": {
   "task_id": "a0",
   "state": "COMPLETED"
  }
 }
}
`))
		})

		t.Run(`CAS fetch error`, func(t *ftt.Test) {
			mockedState["a0"] = swarmingv2.TaskState_COMPLETED
			mockedHasCAS["a0"] = true
			mockedCASErr = errors.New("boom")

			_, code, stdout, _ := SubcommandTest(
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
			assert.Loosely(t, code, should.BeZero)
			assert.Loosely(t, sortJSON(stdout), should.Equal(`{
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
}
`))
		})

		t.Run(`Timeout waiting`, func(t *ftt.Test) {
			mockedState["a0"] = swarmingv2.TaskState_PENDING

			_, code, stdout, _ := SubcommandTest(
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
			assert.Loosely(t, code, should.BeZero)
			assert.Loosely(t, sortJSON(stdout), should.Equal(`{
 "a0": {
  "error": "rpc_timeout",
  "results": {
   "task_id": "a0",
   "state": "PENDING"
  }
 }
}
`))
		})

		t.Run(`No waiting`, func(t *ftt.Test) {
			mockedState["a0"] = swarmingv2.TaskState_PENDING
			mockedState["a1"] = swarmingv2.TaskState_PENDING

			onSleep = func() {
				panic("must not sleep")
			}

			_, code, stdout, _ := SubcommandTest(
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
			assert.Loosely(t, code, should.BeZero)
			assert.Loosely(t, sortJSON(stdout), should.Equal(`{
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
}
`))
		})

		t.Run(`Waiting any`, func(t *ftt.Test) {
			mockedState["a0"] = swarmingv2.TaskState_PENDING
			mockedState["a1"] = swarmingv2.TaskState_PENDING
			mockedState["a2"] = swarmingv2.TaskState_PENDING

			onSleep = func() {
				mockedState["a1"] = swarmingv2.TaskState_COMPLETED
				mockedState["a2"] = swarmingv2.TaskState_COMPLETED
			}

			_, code, stdout, _ := SubcommandTest(
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
			assert.Loosely(t, code, should.BeZero)
			assert.Loosely(t, sortJSON(stdout), should.Equal(`{
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
}
`))
		})
	})
}

func TestCollectSummarizeResults(t *testing.T) {
	t.Parallel()

	ftt.Run(`Generates json.`, t, func(t *ftt.Test) {
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

		tmpDir := t.TempDir()

		emitted := passThroughEmitter(t, func(sink *output.Sink) summaryEmitter {
			return &defaultSummaryEmitter{
				sink:           sink,
				populateStdout: true,
			}
		}, []*taskResult{
			{
				taskID: "task1",
				result: result1,
				output: fakeTextOutput(t, tmpDir, "Output"),
			},
			{
				taskID: "task2",
				result: result2,
				output: fakeTextOutput(t, tmpDir, "Output"),
			},
			{
				taskID: "task3",
				result: result3,
				output: fakeTextOutput(t, tmpDir, "Output"),
			},
			{
				taskID: "task4",
				result: result4,
				output: fakeTextOutput(t, tmpDir, "Output"),
			},
		})

		assert.Loosely(t, emitted, should.Equal(`{
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
}
`))
	})
}

func TestCollectSummarizeResultsPython(t *testing.T) {
	t.Parallel()

	ftt.Run(`Simple json.`, t, func(t *ftt.Test) {
		tmpDir := t.TempDir()

		emitted := passThroughEmitter(t, func(sink *output.Sink) summaryEmitter {
			return &legacySummaryEmitter{
				sink:           sink,
				populateStdout: true,
				taskIDs:        []string{"failed1", "finished", "failed2"},
				resultByID:     map[string]*taskResult{},
			}
		}, []*taskResult{
			{
				taskID: "finished",
				result: &swarmingv2.TaskResultResponse{
					State:    swarmingv2.TaskState_COMPLETED,
					Duration: 1,
					ExitCode: 0,
				},
				output: fakeTextOutput(t, tmpDir, "Output"),
			},
			{
				taskID: "failed1",
				err:    errors.New("boom"),
			},
			{
				taskID: "failed2",
				err:    errors.New("boom"),
			},
		})

		assert.Loosely(t, emitted, should.Equal(`{
 "shards": [
  null,
  {
   "duration": 1,
   "output": "Output",
   "state": "COMPLETED"
  },
  null
 ]
}
`))
	})
}

func sortJSON(s string) string {
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(s), &top); err != nil {
		panic(err)
	}
	blob, err := json.MarshalIndent(top, "", " ")
	if err != nil {
		panic(err)
	}
	return string(blob) + "\n"
}

func passThroughEmitter(t testing.TB, emitter func(sink *output.Sink) summaryEmitter, results []*taskResult) string {
	t.Helper()
	var merr errors.MultiError
	var buf bytes.Buffer
	sink := output.NewSink(&buf)
	em := emitter(sink)
	em.start(&merr)
	for _, res := range results {
		em.emit(res, &merr)
	}
	em.finish(&merr)
	assert.Loosely(t, merr, should.BeEmpty, truth.LineContext())
	assert.Loosely(t, sink.Finalize(), should.BeNil, truth.LineContext())
	return buf.String()
}

func fakeTextOutput(t testing.TB, tmpDir string, text string) *textOutput {
	t.Helper()
	file, err := os.CreateTemp(tmpDir, "swarming_collect_test_*.txt")
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	_, err = file.WriteString(text)
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	return &textOutput{
		file: file,
		temp: true,
	}
}
