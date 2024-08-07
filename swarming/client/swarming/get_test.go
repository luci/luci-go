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

package swarming

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	swarmingv2 "go.chromium.org/luci/swarming/proto/api_v2"
)

func TestGetOne(t *testing.T) {
	t.Parallel()

	ftt.Run("With context", t, func(t *ftt.Test) {
		sleeps := 0
		contextTimeout := 2 * time.Hour
		ctx := mockContext(&sleeps, &contextTimeout, nil)

		const doneTaskID = "done"
		const doneLaterID = "later"
		const doneNever = "never"
		const fatalTaskID = "fatal"

		client := clientMock{
			taskResultMock: func(ctx context.Context, taskID string, fields *TaskResultFields) (*swarmingv2.TaskResultResponse, error) {
				state := swarmingv2.TaskState_PENDING
				switch {
				case taskID == doneTaskID:
					state = swarmingv2.TaskState_COMPLETED
				case taskID == doneLaterID && clock.Since(ctx, testclock.TestRecentTimeUTC) > time.Hour:
					state = swarmingv2.TaskState_COMPLETED
				case taskID == fatalTaskID:
					return nil, status.Errorf(codes.PermissionDenied, "boo")
				}
				return &swarmingv2.TaskResultResponse{
					TaskId: taskID,
					State:  state,
				}, nil
			},
		}

		t.Run("Already done task: Wait", func(t *ftt.Test) {
			res, err := GetOne(ctx, client, doneTaskID, nil, WaitAll)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.State, should.Equal(swarmingv2.TaskState_COMPLETED))
			assert.Loosely(t, sleeps, should.BeZero)
		})

		t.Run("Already done task: Poll", func(t *ftt.Test) {
			res, err := GetOne(ctx, client, doneTaskID, nil, NoWait)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.State, should.Equal(swarmingv2.TaskState_COMPLETED))
			assert.Loosely(t, sleeps, should.BeZero)
		})

		t.Run("Pending task: Wait", func(t *ftt.Test) {
			res, err := GetOne(ctx, client, doneLaterID, nil, WaitAll)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.State, should.Equal(swarmingv2.TaskState_COMPLETED))
			assert.Loosely(t, sleeps, should.Equal(254)) // ~= 1h / 15s sans ramp up and random
		})

		t.Run("Pending task: Poll", func(t *ftt.Test) {
			res, err := GetOne(ctx, client, doneLaterID, nil, NoWait)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.State, should.Equal(swarmingv2.TaskState_PENDING))
			assert.Loosely(t, sleeps, should.BeZero)
		})

		t.Run("Context canceled while waiting", func(t *ftt.Test) {
			res, err := GetOne(ctx, client, doneNever, nil, WaitAll)
			assert.Loosely(t, err, should.Equal(context.Canceled))
			assert.Loosely(t, res.State, should.Equal(swarmingv2.TaskState_PENDING))
			assert.Loosely(t, sleeps, should.Equal(459)) // ~= 2h / 15s sans ramp up and random
		})

		t.Run("Fatal RPC failure", func(t *ftt.Test) {
			res, err := GetOne(ctx, client, fatalTaskID, nil, WaitAll)
			assert.Loosely(t, status.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, res, should.BeNil)
			assert.Loosely(t, sleeps, should.BeZero)
		})
	})
}

func TestGetMany(t *testing.T) {
	t.Parallel()

	ftt.Run("With context", t, func(t *ftt.Test) {
		sleeps := 0
		contextTimeout := 2 * time.Hour
		onSleep := func() {}
		ctx := mockContext(&sleeps, &contextTimeout, func() { onSleep() })

		mockedState := map[string]swarmingv2.TaskState{}
		mockedErr := map[string]codes.Code{}
		mockedRPCErr := codes.OK

		client := clientMock{
			taskResultsMock: func(ctx context.Context, taskIDs []string, fields *TaskResultFields) ([]ResultOrErr, error) {
				if mockedRPCErr != codes.OK {
					return nil, status.Errorf(mockedRPCErr, "RPC error")
				}
				out := make([]ResultOrErr, len(taskIDs))
				for i, taskID := range taskIDs {
					if code, ok := mockedErr[taskID]; ok {
						out[i] = ResultOrErr{Err: status.Errorf(code, "some error")}
					} else if state, ok := mockedState[taskID]; ok {
						out[i] = ResultOrErr{
							Result: &swarmingv2.TaskResultResponse{
								TaskId: taskID,
								State:  state,
							},
						}
					} else {
						panic(fmt.Sprintf("unexpected task %q", taskID))
					}
				}
				return out, nil
			},
		}

		type stateOrErr struct {
			state swarmingv2.TaskState
			err   codes.Code
		}

		call := func(ctx context.Context, taskIDs []string, mode WaitMode) map[string]stateOrErr {
			out := map[string]stateOrErr{}
			GetMany(ctx, client, taskIDs, nil, mode, func(taskID string, res *swarmingv2.TaskResultResponse, err error) {
				if _, ok := out[taskID]; ok {
					panic(fmt.Sprintf("task %q reported twice", taskID))
				}
				s := stateOrErr{}
				if err != nil {
					if err == context.Canceled || err == context.DeadlineExceeded {
						s.err = codes.DeadlineExceeded
					} else {
						s.err = status.Code(err)
					}
				}
				if res != nil {
					s.state = res.State
				}
				out[taskID] = s
			})
			assert.Loosely(t, len(out), should.Equal(len(taskIDs)))
			return out
		}

		t.Run("NoWait: no RPC errors", func(t *ftt.Test) {
			mockedState["done"] = swarmingv2.TaskState_COMPLETED
			mockedState["pending"] = swarmingv2.TaskState_PENDING
			mockedErr["missing"] = codes.NotFound
			out := call(ctx, []string{"done", "pending", "missing"}, NoWait)
			assert.Loosely(t, out, should.Resemble(map[string]stateOrErr{
				"done":    {state: swarmingv2.TaskState_COMPLETED},
				"pending": {state: swarmingv2.TaskState_PENDING},
				"missing": {err: codes.NotFound},
			}))
		})

		t.Run("NoWait: fatal RPC error", func(t *ftt.Test) {
			mockedRPCErr = codes.PermissionDenied
			out := call(ctx, []string{"t1", "t2"}, NoWait)
			assert.Loosely(t, out, should.Resemble(map[string]stateOrErr{
				"t1": {err: codes.PermissionDenied},
				"t2": {err: codes.PermissionDenied},
			}))
		})

		t.Run("NoWait: transient RPC errors", func(t *ftt.Test) {
			mockedState["done"] = swarmingv2.TaskState_COMPLETED
			mockedState["pending"] = swarmingv2.TaskState_PENDING

			mockedRPCErr = codes.Internal
			onSleep = func() {
				if sleeps > 5 {
					mockedRPCErr = codes.OK
				}
			}

			out := call(ctx, []string{"done", "pending"}, NoWait)
			assert.Loosely(t, out, should.Resemble(map[string]stateOrErr{
				"done":    {state: swarmingv2.TaskState_COMPLETED},
				"pending": {state: swarmingv2.TaskState_PENDING},
			}))
			assert.Loosely(t, sleeps, should.Equal(6))
		})

		t.Run("WaitAll: no RPC errors", func(t *ftt.Test) {
			mockedState["t1"] = swarmingv2.TaskState_COMPLETED
			mockedState["t2"] = swarmingv2.TaskState_PENDING
			mockedState["t3"] = swarmingv2.TaskState_PENDING
			mockedState["t4"] = swarmingv2.TaskState_PENDING

			onSleep = func() {
				switch sleeps {
				case 2:
					mockedErr["t2"] = codes.NotFound
				case 3:
					mockedState["t3"] = swarmingv2.TaskState_COMPLETED
				case 4:
					mockedState["t4"] = swarmingv2.TaskState_COMPLETED
				}
			}

			out := call(ctx, []string{"t1", "t2", "t3", "t4"}, WaitAll)
			assert.Loosely(t, out, should.Resemble(map[string]stateOrErr{
				"t1": {state: swarmingv2.TaskState_COMPLETED},
				"t2": {state: swarmingv2.TaskState_PENDING, err: codes.NotFound},
				"t3": {state: swarmingv2.TaskState_COMPLETED},
				"t4": {state: swarmingv2.TaskState_COMPLETED},
			}))
			assert.Loosely(t, sleeps, should.Equal(4))
		})

		t.Run("WaitAll: fatal RPC error", func(t *ftt.Test) {
			mockedState["t1"] = swarmingv2.TaskState_COMPLETED
			mockedState["t2"] = swarmingv2.TaskState_PENDING
			mockedState["t3"] = swarmingv2.TaskState_PENDING
			mockedState["t4"] = swarmingv2.TaskState_PENDING

			onSleep = func() {
				switch sleeps {
				case 2:
					mockedState["t4"] = swarmingv2.TaskState_COMPLETED
				case 3:
					mockedRPCErr = codes.PermissionDenied
				}
			}

			out := call(ctx, []string{"t1", "t2", "t3", "t4"}, WaitAll)
			assert.Loosely(t, out, should.Resemble(map[string]stateOrErr{
				"t1": {state: swarmingv2.TaskState_COMPLETED},
				"t2": {state: swarmingv2.TaskState_PENDING, err: codes.PermissionDenied},
				"t3": {state: swarmingv2.TaskState_PENDING, err: codes.PermissionDenied},
				"t4": {state: swarmingv2.TaskState_COMPLETED},
			}))
			assert.Loosely(t, sleeps, should.Equal(3))
		})

		t.Run("WaitAll: transient RPC errors", func(t *ftt.Test) {
			mockedState["t1"] = swarmingv2.TaskState_COMPLETED
			mockedState["t2"] = swarmingv2.TaskState_PENDING
			mockedState["t3"] = swarmingv2.TaskState_PENDING
			mockedState["t4"] = swarmingv2.TaskState_PENDING

			onSleep = func() {
				switch sleeps {
				case 5:
					mockedRPCErr = codes.Internal
				case 8:
					mockedRPCErr = codes.OK
				case 10:
					mockedErr["t2"] = codes.NotFound
				case 11:
					mockedState["t3"] = swarmingv2.TaskState_COMPLETED
				case 12:
					mockedState["t4"] = swarmingv2.TaskState_COMPLETED
				}
			}

			out := call(ctx, []string{"t1", "t2", "t3", "t4"}, WaitAll)
			assert.Loosely(t, out, should.Resemble(map[string]stateOrErr{
				"t1": {state: swarmingv2.TaskState_COMPLETED},
				"t2": {state: swarmingv2.TaskState_PENDING, err: codes.NotFound},
				"t3": {state: swarmingv2.TaskState_COMPLETED},
				"t4": {state: swarmingv2.TaskState_COMPLETED},
			}))
			assert.Loosely(t, sleeps, should.Equal(12))
		})

		t.Run("WaitAll: context deadline", func(t *ftt.Test) {
			mockedState["t1"] = swarmingv2.TaskState_COMPLETED
			mockedState["t2"] = swarmingv2.TaskState_PENDING

			out := call(ctx, []string{"t1", "t2"}, WaitAll)
			assert.Loosely(t, out, should.Resemble(map[string]stateOrErr{
				"t1": {state: swarmingv2.TaskState_COMPLETED},
				"t2": {state: swarmingv2.TaskState_PENDING, err: codes.DeadlineExceeded},
			}))
			assert.Loosely(t, sleeps, should.Equal(459))
		})

		t.Run("WaitAny: no RPC errors, no waiting", func(t *ftt.Test) {
			mockedState["t1"] = swarmingv2.TaskState_COMPLETED
			mockedState["t2"] = swarmingv2.TaskState_PENDING

			out := call(ctx, []string{"t1", "t2"}, WaitAny)
			assert.Loosely(t, out, should.Resemble(map[string]stateOrErr{
				"t1": {state: swarmingv2.TaskState_COMPLETED},
				"t2": {state: swarmingv2.TaskState_PENDING},
			}))
			assert.Loosely(t, sleeps, should.BeZero)
		})

		t.Run("WaitAny: no RPC errors, waiting", func(t *ftt.Test) {
			mockedState["t1"] = swarmingv2.TaskState_PENDING
			mockedState["t2"] = swarmingv2.TaskState_PENDING
			mockedState["t3"] = swarmingv2.TaskState_PENDING
			mockedState["t4"] = swarmingv2.TaskState_PENDING

			onSleep = func() {
				switch sleeps {
				case 5:
					mockedErr["t2"] = codes.NotFound
				case 6:
					mockedState["t1"] = swarmingv2.TaskState_COMPLETED
					mockedState["t4"] = swarmingv2.TaskState_COMPLETED
				}
			}

			out := call(ctx, []string{"t1", "t2", "t3", "t4"}, WaitAny)
			assert.Loosely(t, out, should.Resemble(map[string]stateOrErr{
				"t1": {state: swarmingv2.TaskState_COMPLETED},
				"t2": {state: swarmingv2.TaskState_PENDING, err: codes.NotFound},
				"t3": {state: swarmingv2.TaskState_PENDING},
				"t4": {state: swarmingv2.TaskState_COMPLETED},
			}))
			assert.Loosely(t, sleeps, should.Equal(6))
		})

		t.Run("WaitAny: fatal RPC error", func(t *ftt.Test) {
			mockedState["t1"] = swarmingv2.TaskState_PENDING
			mockedState["t2"] = swarmingv2.TaskState_PENDING
			mockedState["t3"] = swarmingv2.TaskState_PENDING
			mockedState["t4"] = swarmingv2.TaskState_PENDING

			onSleep = func() {
				switch sleeps {
				case 5:
					mockedErr["t2"] = codes.NotFound
				case 6:
					mockedRPCErr = codes.PermissionDenied
				}
			}

			out := call(ctx, []string{"t1", "t2", "t3", "t4"}, WaitAny)
			assert.Loosely(t, out, should.Resemble(map[string]stateOrErr{
				"t1": {state: swarmingv2.TaskState_PENDING, err: codes.PermissionDenied},
				"t2": {state: swarmingv2.TaskState_PENDING, err: codes.NotFound},
				"t3": {state: swarmingv2.TaskState_PENDING, err: codes.PermissionDenied},
				"t4": {state: swarmingv2.TaskState_PENDING, err: codes.PermissionDenied},
			}))
			assert.Loosely(t, sleeps, should.Equal(6))
		})

		t.Run("WaitAny: transient RPC errors", func(t *ftt.Test) {
			mockedState["t1"] = swarmingv2.TaskState_PENDING
			mockedState["t2"] = swarmingv2.TaskState_PENDING

			mockedRPCErr = codes.Internal

			onSleep = func() {
				switch sleeps {
				case 5:
					mockedRPCErr = codes.OK
				case 10:
					mockedState["t2"] = swarmingv2.TaskState_COMPLETED
				}
			}

			out := call(ctx, []string{"t1", "t2"}, WaitAny)
			assert.Loosely(t, out, should.Resemble(map[string]stateOrErr{
				"t1": {state: swarmingv2.TaskState_PENDING},
				"t2": {state: swarmingv2.TaskState_COMPLETED},
			}))
			assert.Loosely(t, sleeps, should.Equal(10))
		})

		t.Run("WaitAny: context deadline", func(t *ftt.Test) {
			mockedState["t1"] = swarmingv2.TaskState_PENDING
			mockedState["t2"] = swarmingv2.TaskState_PENDING

			out := call(ctx, []string{"t1", "t2"}, WaitAny)
			assert.Loosely(t, out, should.Resemble(map[string]stateOrErr{
				"t1": {state: swarmingv2.TaskState_PENDING, err: codes.DeadlineExceeded},
				"t2": {state: swarmingv2.TaskState_PENDING, err: codes.DeadlineExceeded},
			}))
			assert.Loosely(t, sleeps, should.Equal(459))
		})
	})
}

func mockContext(sleeps *int, timeout *time.Duration, cb func()) context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	ctx = mathrand.Set(ctx, rand.New(rand.NewSource(0)))

	ctx, clk := testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
	clk.SetTimerCallback(func(d time.Duration, t clock.Timer) {
		if clk.Now().Sub(testclock.TestRecentTimeUTC) > *timeout {
			cancel()
		} else {
			if sleeps != nil {
				*sleeps += 1
			}
			if cb != nil {
				cb()
			}
			clk.Add(d)
		}
	})

	return ctx
}

// Unfortunately we can't use swarmingtest.Client here due to module import
// cycles, so set up a separate more targeted mock.
type clientMock struct {
	Client // to "implement" all other methods by panicing with nil dereference

	taskResultMock  func(ctx context.Context, taskID string, fields *TaskResultFields) (*swarmingv2.TaskResultResponse, error)
	taskResultsMock func(ctx context.Context, taskIDs []string, fields *TaskResultFields) ([]ResultOrErr, error)
}

func (m clientMock) TaskResult(ctx context.Context, taskID string, fields *TaskResultFields) (*swarmingv2.TaskResultResponse, error) {
	return m.taskResultMock(ctx, taskID, fields)
}

func (m clientMock) TaskResults(ctx context.Context, taskIDs []string, fields *TaskResultFields) ([]ResultOrErr, error) {
	return m.taskResultsMock(ctx, taskIDs, fields)
}
