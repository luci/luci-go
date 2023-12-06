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

	swarmingv2 "go.chromium.org/luci/swarming/proto/api_v2"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGetOne(t *testing.T) {
	t.Parallel()

	Convey("With context", t, func() {
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

		Convey("Already done task: Wait", func() {
			res, err := GetOne(ctx, client, doneTaskID, nil, WaitAll)
			So(err, ShouldBeNil)
			So(res.State, ShouldEqual, swarmingv2.TaskState_COMPLETED)
			So(sleeps, ShouldEqual, 0)
		})

		Convey("Already done task: Poll", func() {
			res, err := GetOne(ctx, client, doneTaskID, nil, NoWait)
			So(err, ShouldBeNil)
			So(res.State, ShouldEqual, swarmingv2.TaskState_COMPLETED)
			So(sleeps, ShouldEqual, 0)
		})

		Convey("Pending task: Wait", func() {
			res, err := GetOne(ctx, client, doneLaterID, nil, WaitAll)
			So(err, ShouldBeNil)
			So(res.State, ShouldEqual, swarmingv2.TaskState_COMPLETED)
			So(sleeps, ShouldEqual, 254) // ~= 1h / 15s sans ramp up and random
		})

		Convey("Pending task: Poll", func() {
			res, err := GetOne(ctx, client, doneLaterID, nil, NoWait)
			So(err, ShouldBeNil)
			So(res.State, ShouldEqual, swarmingv2.TaskState_PENDING)
			So(sleeps, ShouldEqual, 0)
		})

		Convey("Context canceled while waiting", func() {
			res, err := GetOne(ctx, client, doneNever, nil, WaitAll)
			So(err, ShouldEqual, context.Canceled)
			So(res.State, ShouldEqual, swarmingv2.TaskState_PENDING)
			So(sleeps, ShouldEqual, 459) // ~= 2h / 15s sans ramp up and random
		})

		Convey("Fatal RPC failure", func() {
			res, err := GetOne(ctx, client, fatalTaskID, nil, WaitAll)
			So(status.Code(err), ShouldEqual, codes.PermissionDenied)
			So(res, ShouldBeNil)
			So(sleeps, ShouldEqual, 0)
		})
	})
}

func TestGetMany(t *testing.T) {
	t.Parallel()

	Convey("With context", t, func() {
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
			So(len(out), ShouldEqual, len(taskIDs))
			return out
		}

		Convey("NoWait: no RPC errors", func() {
			mockedState["done"] = swarmingv2.TaskState_COMPLETED
			mockedState["pending"] = swarmingv2.TaskState_PENDING
			mockedErr["missing"] = codes.NotFound
			out := call(ctx, []string{"done", "pending", "missing"}, NoWait)
			So(out, ShouldResemble, map[string]stateOrErr{
				"done":    {state: swarmingv2.TaskState_COMPLETED},
				"pending": {state: swarmingv2.TaskState_PENDING},
				"missing": {err: codes.NotFound},
			})
		})

		Convey("NoWait: fatal RPC error", func() {
			mockedRPCErr = codes.PermissionDenied
			out := call(ctx, []string{"t1", "t2"}, NoWait)
			So(out, ShouldResemble, map[string]stateOrErr{
				"t1": {err: codes.PermissionDenied},
				"t2": {err: codes.PermissionDenied},
			})
		})

		Convey("NoWait: transient RPC errors", func() {
			mockedState["done"] = swarmingv2.TaskState_COMPLETED
			mockedState["pending"] = swarmingv2.TaskState_PENDING

			mockedRPCErr = codes.Internal
			onSleep = func() {
				if sleeps > 5 {
					mockedRPCErr = codes.OK
				}
			}

			out := call(ctx, []string{"done", "pending"}, NoWait)
			So(out, ShouldResemble, map[string]stateOrErr{
				"done":    {state: swarmingv2.TaskState_COMPLETED},
				"pending": {state: swarmingv2.TaskState_PENDING},
			})
			So(sleeps, ShouldEqual, 6)
		})

		Convey("WaitAll: no RPC errors", func() {
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
			So(out, ShouldResemble, map[string]stateOrErr{
				"t1": {state: swarmingv2.TaskState_COMPLETED},
				"t2": {state: swarmingv2.TaskState_PENDING, err: codes.NotFound},
				"t3": {state: swarmingv2.TaskState_COMPLETED},
				"t4": {state: swarmingv2.TaskState_COMPLETED},
			})
			So(sleeps, ShouldEqual, 4)
		})

		Convey("WaitAll: fatal RPC error", func() {
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
			So(out, ShouldResemble, map[string]stateOrErr{
				"t1": {state: swarmingv2.TaskState_COMPLETED},
				"t2": {state: swarmingv2.TaskState_PENDING, err: codes.PermissionDenied},
				"t3": {state: swarmingv2.TaskState_PENDING, err: codes.PermissionDenied},
				"t4": {state: swarmingv2.TaskState_COMPLETED},
			})
			So(sleeps, ShouldEqual, 3)
		})

		Convey("WaitAll: transient RPC errors", func() {
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
			So(out, ShouldResemble, map[string]stateOrErr{
				"t1": {state: swarmingv2.TaskState_COMPLETED},
				"t2": {state: swarmingv2.TaskState_PENDING, err: codes.NotFound},
				"t3": {state: swarmingv2.TaskState_COMPLETED},
				"t4": {state: swarmingv2.TaskState_COMPLETED},
			})
			So(sleeps, ShouldEqual, 12)
		})

		Convey("WaitAll: context deadline", func() {
			mockedState["t1"] = swarmingv2.TaskState_COMPLETED
			mockedState["t2"] = swarmingv2.TaskState_PENDING

			out := call(ctx, []string{"t1", "t2"}, WaitAll)
			So(out, ShouldResemble, map[string]stateOrErr{
				"t1": {state: swarmingv2.TaskState_COMPLETED},
				"t2": {state: swarmingv2.TaskState_PENDING, err: codes.DeadlineExceeded},
			})
			So(sleeps, ShouldEqual, 459)
		})

		Convey("WaitAny: no RPC errors, no waiting", func() {
			mockedState["t1"] = swarmingv2.TaskState_COMPLETED
			mockedState["t2"] = swarmingv2.TaskState_PENDING

			out := call(ctx, []string{"t1", "t2"}, WaitAny)
			So(out, ShouldResemble, map[string]stateOrErr{
				"t1": {state: swarmingv2.TaskState_COMPLETED},
				"t2": {state: swarmingv2.TaskState_PENDING},
			})
			So(sleeps, ShouldEqual, 0)
		})

		Convey("WaitAny: no RPC errors, waiting", func() {
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
			So(out, ShouldResemble, map[string]stateOrErr{
				"t1": {state: swarmingv2.TaskState_COMPLETED},
				"t2": {state: swarmingv2.TaskState_PENDING, err: codes.NotFound},
				"t3": {state: swarmingv2.TaskState_PENDING},
				"t4": {state: swarmingv2.TaskState_COMPLETED},
			})
			So(sleeps, ShouldEqual, 6)
		})

		Convey("WaitAny: fatal RPC error", func() {
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
			So(out, ShouldResemble, map[string]stateOrErr{
				"t1": {state: swarmingv2.TaskState_PENDING, err: codes.PermissionDenied},
				"t2": {state: swarmingv2.TaskState_PENDING, err: codes.NotFound},
				"t3": {state: swarmingv2.TaskState_PENDING, err: codes.PermissionDenied},
				"t4": {state: swarmingv2.TaskState_PENDING, err: codes.PermissionDenied},
			})
			So(sleeps, ShouldEqual, 6)
		})

		Convey("WaitAny: transient RPC errors", func() {
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
			So(out, ShouldResemble, map[string]stateOrErr{
				"t1": {state: swarmingv2.TaskState_PENDING},
				"t2": {state: swarmingv2.TaskState_COMPLETED},
			})
			So(sleeps, ShouldEqual, 10)
		})

		Convey("WaitAny: context deadline", func() {
			mockedState["t1"] = swarmingv2.TaskState_PENDING
			mockedState["t2"] = swarmingv2.TaskState_PENDING

			out := call(ctx, []string{"t1", "t2"}, WaitAny)
			So(out, ShouldResemble, map[string]stateOrErr{
				"t1": {state: swarmingv2.TaskState_PENDING, err: codes.DeadlineExceeded},
				"t2": {state: swarmingv2.TaskState_PENDING, err: codes.DeadlineExceeded},
			})
			So(sleeps, ShouldEqual, 459)
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
