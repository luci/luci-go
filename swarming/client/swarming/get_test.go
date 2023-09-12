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
	"math/rand"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/mathrand"

	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/luci/swarming/client/swarming/swarmingtest"
	swarmingv2 "go.chromium.org/luci/swarming/proto/api_v2"
)

func TestGetOne(t *testing.T) {
	t.Parallel()

	Convey("With context", t, func() {
		sleeps := 0
		contextTimeout := 2 * time.Hour
		ctx := mockContext("", &sleeps, &contextTimeout)

		const doneTaskID = "done"
		const doneLaterID = "later"
		const doneNever = "never"
		const fatalTaskID = "fatal"
		const wrongTaskID = "wrong"

		client := swarmingtest.Client{
			TaskResultMock: func(ctx context.Context, taskID string, perf bool) (*swarmingv2.TaskResultResponse, error) {
				state := swarmingv2.TaskState_PENDING
				switch {
				case taskID == doneTaskID:
					state = swarmingv2.TaskState_COMPLETED
				case taskID == doneLaterID && clock.Since(ctx, testclock.TestRecentTimeUTC) > time.Hour:
					state = swarmingv2.TaskState_COMPLETED
				case taskID == fatalTaskID:
					return nil, status.Errorf(codes.PermissionDenied, "boo")
				case taskID == wrongTaskID:
					return &swarmingv2.TaskResultResponse{
						TaskId: "incorrect",
						State:  swarmingv2.TaskState_COMPLETED,
					}, nil
				}
				return &swarmingv2.TaskResultResponse{
					TaskId: taskID,
					State:  state,
				}, nil
			},
		}

		Convey("Already done task: Wait", func() {
			res, err := GetOne(ctx, &client, doneTaskID, false, Wait)
			So(err, ShouldBeNil)
			So(res.State, ShouldEqual, swarmingv2.TaskState_COMPLETED)
			So(sleeps, ShouldEqual, 0)
		})

		Convey("Already done task: Poll", func() {
			res, err := GetOne(ctx, &client, doneTaskID, false, Poll)
			So(err, ShouldBeNil)
			So(res.State, ShouldEqual, swarmingv2.TaskState_COMPLETED)
			So(sleeps, ShouldEqual, 0)
		})

		Convey("Pending task: Wait", func() {
			res, err := GetOne(ctx, &client, doneLaterID, false, Wait)
			So(err, ShouldBeNil)
			So(res.State, ShouldEqual, swarmingv2.TaskState_COMPLETED)
			So(sleeps, ShouldEqual, 254) // ~= 1h / 15s sans ramp up and random
		})

		Convey("Pending task: Poll", func() {
			res, err := GetOne(ctx, &client, doneLaterID, false, Poll)
			So(err, ShouldBeNil)
			So(res.State, ShouldEqual, swarmingv2.TaskState_PENDING)
			So(sleeps, ShouldEqual, 0)
		})

		Convey("Context canceled while waiting", func() {
			res, err := GetOne(ctx, &client, doneNever, false, Wait)
			So(err, ShouldEqual, context.Canceled)
			So(res, ShouldBeNil)
			So(sleeps, ShouldEqual, 459) // ~= 2h / 15s sans ramp up and random
		})

		Convey("Fatal RPC failure", func() {
			res, err := GetOne(ctx, &client, fatalTaskID, false, Wait)
			So(status.Code(err), ShouldEqual, codes.PermissionDenied)
			So(res, ShouldBeNil)
			So(sleeps, ShouldEqual, 0)
		})

		Convey("Wrong task ID", func() {
			res, err := GetOne(ctx, &client, wrongTaskID, false, Wait)
			So(status.Code(err), ShouldEqual, codes.FailedPrecondition)
			So(res, ShouldBeNil)
			So(sleeps, ShouldEqual, 0)
		})
	})
}

func TestGetAll(t *testing.T) {
	t.Parallel()

	Convey("With context", t, func() {
		completionTimes := map[string]time.Duration{
			"0":     0,
			"1":     time.Hour,
			"2":     2 * time.Hour,
			"error": 2 * time.Hour,
			"slow":  5 * time.Hour,
		}

		// Advance mock time based on the longest living timer.
		contextTimeout := 10 * time.Hour
		ctx := mockContext("wait-slow", nil, &contextTimeout)

		client := swarmingtest.Client{
			TaskResultMock: func(ctx context.Context, taskID string, perf bool) (*swarmingv2.TaskResultResponse, error) {
				state := swarmingv2.TaskState_PENDING
				if clock.Since(ctx, testclock.TestRecentTimeUTC) >= completionTimes[taskID] {
					if taskID == "error" {
						return nil, status.Errorf(codes.PermissionDenied, "boo")
					}
					state = swarmingv2.TaskState_COMPLETED
				}
				return &swarmingv2.TaskResultResponse{
					TaskId: taskID,
					State:  state,
				}, nil
			},
		}

		call := func(taskIDs []string, mode BlockMode) map[string]string {
			out := map[string]string{}
			m := sync.Mutex{}
			GetAll(ctx, &client, taskIDs, false, mode, func(taskID string, resp *swarmingv2.TaskResultResponse, err error) {
				m.Lock()
				defer m.Unlock()
				if _, dup := out[taskID]; dup {
					panic("visited the same task twice!")
				}
				if resp != nil && resp.TaskId != taskID {
					panic("wrong task ID in result")
				}
				if err == nil {
					out[taskID] = resp.State.String()
				} else {
					out[taskID] = err.Error()
				}
			})
			return out
		}

		Convey("Success: Wait", func() {
			out := call([]string{"0", "1", "2", "slow"}, Wait)
			So(out, ShouldResemble, map[string]string{
				"0":    "COMPLETED",
				"1":    "COMPLETED",
				"2":    "COMPLETED",
				"slow": "COMPLETED",
			})
		})

		Convey("Success: Poll", func() {
			out := call([]string{"0", "1", "2", "slow"}, Poll)
			So(out, ShouldResemble, map[string]string{
				"0":    "COMPLETED",
				"1":    "PENDING",
				"2":    "PENDING",
				"slow": "PENDING",
			})
		})

		Convey("Timeout", func() {
			contextTimeout = time.Hour + 30*time.Minute
			out := call([]string{"0", "1", "2", "slow"}, Wait)
			// Note: due to race conditions in terminating concurrent goroutines, we
			// can't reliably assert that both "0" and "1" have been completed. But
			// "2" should **not** have been completed visited for sure.
			So(out["2"], ShouldEqual, "context canceled")
		})

		Convey("RPC error", func() {
			out := call([]string{"error", "0", "1", "slow"}, Wait)
			So(out, ShouldResemble, map[string]string{
				"error": "rpc error: code = PermissionDenied desc = boo",
				"0":     "COMPLETED",
				"1":     "COMPLETED",
				"slow":  "COMPLETED",
			})
		})
	})
}

func TestGetAnyCompleted(t *testing.T) {
	t.Parallel()

	Convey("With context", t, func() {
		completionTimes := map[string]time.Duration{
			"0":     0,
			"1":     time.Hour,
			"error": 2 * time.Hour,
		}

		// Advance mock time based on the longest living timer.
		contextTimeout := 10 * time.Hour
		ctx := mockContext("wait-stuck", nil, &contextTimeout)

		client := swarmingtest.Client{
			TaskResultMock: func(ctx context.Context, taskID string, perf bool) (*swarmingv2.TaskResultResponse, error) {
				state := swarmingv2.TaskState_PENDING
				if taskID != "stuck" && clock.Since(ctx, testclock.TestRecentTimeUTC) >= completionTimes[taskID] {
					if taskID == "error" {
						return nil, status.Errorf(codes.PermissionDenied, "boo")
					}
					state = swarmingv2.TaskState_COMPLETED
				}
				return &swarmingv2.TaskResultResponse{
					TaskId: taskID,
					State:  state,
				}, nil
			},
		}

		Convey("Success: Wait", func() {
			taskID, res, err := GetAnyCompleted(ctx, &client, []string{"stuck", "1"}, false, Wait)
			So(err, ShouldBeNil)
			So(taskID, ShouldEqual, "1")
			So(res.TaskId, ShouldEqual, "1")
		})

		Convey("Success: Poll", func() {
			taskID, res, err := GetAnyCompleted(ctx, &client, []string{"0", "stuck"}, false, Poll)
			So(err, ShouldBeNil)
			So(taskID, ShouldEqual, "0")
			So(res.TaskId, ShouldEqual, "0")
		})

		Convey("Success: Poll with all pending", func() {
			taskID, _, err := GetAnyCompleted(ctx, &client, []string{"stuck", "1"}, false, Poll)
			So(err, ShouldEqual, ErrAllPending)
			So(taskID, ShouldEqual, "")
		})

		Convey("Timeout", func() {
			contextTimeout = 30 * time.Minute
			taskID, res, err := GetAnyCompleted(ctx, &client, []string{"stuck", "1"}, false, Wait)
			So(err, ShouldEqual, context.Canceled)
			So(res, ShouldBeNil)
			So(taskID, ShouldNotEqual, "") // can be either of them
		})

		Convey("RPC error", func() {
			taskID, res, err := GetAnyCompleted(ctx, &client, []string{"stuck", "error"}, false, Wait)
			So(status.Code(err), ShouldEqual, codes.PermissionDenied)
			So(res, ShouldBeNil)
			So(taskID, ShouldEqual, "error")
		})
	})
}

func mockContext(drivingTimer string, sleeps *int, timeout *time.Duration) context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	ctx = mathrand.Set(ctx, rand.New(rand.NewSource(0)))

	ctx, clk := testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
	clk.SetTimerCallback(func(d time.Duration, t clock.Timer) {
		// Advance clock only based on a single running timer. The rest will
		// follow. Advancing clock on ticks of all timers makes no sense, since
		// these timers are running in parallel.
		if drivingTimer == "" || testclock.HasTags(t, drivingTimer) {
			if clk.Now().Sub(testclock.TestRecentTimeUTC) > *timeout {
				cancel()
			} else {
				if sleeps != nil {
					*sleeps += 1
				}
				clk.Add(d)
			}
		}
	})

	return ctx
}
