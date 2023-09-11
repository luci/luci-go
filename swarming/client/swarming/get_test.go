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
		const transientTaskID = "transient"
		const wrongTaskID = "wrong"

		callAttempt := 0

		client := swarmingtest.Client{
			TaskResultMock: func(ctx context.Context, taskID string, perf bool) (*swarmingv2.TaskResultResponse, error) {
				callAttempt += 1
				state := swarmingv2.TaskState_PENDING
				switch {
				case taskID == doneTaskID:
					state = swarmingv2.TaskState_COMPLETED
				case taskID == doneLaterID && clock.Since(ctx, testclock.TestRecentTimeUTC) > time.Hour:
					state = swarmingv2.TaskState_COMPLETED
				case taskID == fatalTaskID:
					return nil, status.Errorf(codes.PermissionDenied, "boo")
				case taskID == transientTaskID:
					if callAttempt <= 100 {
						return nil, status.Errorf(codes.Internal, "boo")
					}
					state = swarmingv2.TaskState_COMPLETED
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

		Convey("Transient RPC failure: Wait", func() {
			res, err := GetOne(ctx, &client, transientTaskID, false, Wait)
			So(err, ShouldBeNil)
			So(res.State, ShouldEqual, swarmingv2.TaskState_COMPLETED)
			So(sleeps, ShouldEqual, 100)
		})

		Convey("Transient RPC failure: Poll", func() {
			res, err := GetOne(ctx, &client, transientTaskID, false, Poll)
			So(err, ShouldBeNil)
			So(res.State, ShouldEqual, swarmingv2.TaskState_COMPLETED)
			So(sleeps, ShouldEqual, 100) // retries just like in Wait mode
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

		call := func(taskIDs []string, mode BlockMode) (map[string]swarmingv2.TaskState, error) {
			out := map[string]swarmingv2.TaskState{}
			m := sync.Mutex{}
			err := GetAll(ctx, &client, taskIDs, false, mode, func(resp *swarmingv2.TaskResultResponse) {
				m.Lock()
				defer m.Unlock()
				if _, dup := out[resp.TaskId]; dup {
					panic("visited the same task twice!")
				}
				out[resp.TaskId] = resp.State
			})
			return out, err
		}

		Convey("Success: Wait", func() {
			out, err := call([]string{"0", "1", "2", "slow"}, Wait)
			So(err, ShouldBeNil)
			So(out, ShouldResemble, map[string]swarmingv2.TaskState{
				"0":    swarmingv2.TaskState_COMPLETED,
				"1":    swarmingv2.TaskState_COMPLETED,
				"2":    swarmingv2.TaskState_COMPLETED,
				"slow": swarmingv2.TaskState_COMPLETED,
			})
		})

		Convey("Success: Poll", func() {
			out, err := call([]string{"0", "1", "2", "slow"}, Poll)
			So(err, ShouldBeNil)
			So(out, ShouldResemble, map[string]swarmingv2.TaskState{
				"0":    swarmingv2.TaskState_COMPLETED,
				"1":    swarmingv2.TaskState_PENDING,
				"2":    swarmingv2.TaskState_PENDING,
				"slow": swarmingv2.TaskState_PENDING,
			})
		})

		Convey("Timeout", func() {
			contextTimeout = time.Hour + 30*time.Minute
			out, err := call([]string{"0", "1", "2", "slow"}, Wait)
			So(err, ShouldEqual, context.Canceled)
			// Note: due to race conditions in terminating concurrent goroutines, we
			// can't reliably assert that both "0" and "1" have been visited. But "2"
			// should **not** have been visited for sure.
			So(out["2"], ShouldEqual, swarmingv2.TaskState_INVALID)
		})

		Convey("RPC error", func() {
			_, err := call([]string{"error", "0", "1", "slow"}, Wait)
			So(status.Code(err), ShouldEqual, codes.PermissionDenied)
		})
	})
}

func TestGetAnyCompleted(t *testing.T) {
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

		Convey("Success: Wait", func() {
			res, err := GetAnyCompleted(ctx, &client, []string{"slow", "1", "2"}, false, Wait)
			So(err, ShouldBeNil)
			// TODO(vadimsh): Theoretically "2" can win the race too, but looks like
			// it never happens.
			So(res.TaskId, ShouldEqual, "1")
		})

		Convey("Success: Poll", func() {
			res, err := GetAnyCompleted(ctx, &client, []string{"0", "slow", "1", "2"}, false, Poll)
			So(err, ShouldBeNil)
			So(res.TaskId, ShouldEqual, "0")
		})

		Convey("Success: Poll with all pending", func() {
			_, err := GetAnyCompleted(ctx, &client, []string{"slow", "1", "2"}, false, Poll)
			So(err, ShouldEqual, ErrAllPending)
		})

		Convey("Timeout", func() {
			contextTimeout = 30 * time.Minute
			res, err := GetAnyCompleted(ctx, &client, []string{"slow", "1", "2"}, false, Wait)
			So(err, ShouldEqual, context.Canceled)
			So(res, ShouldBeNil)
		})

		Convey("RPC error", func() {
			res, err := GetAnyCompleted(ctx, &client, []string{"slow", "error"}, false, Wait)
			So(status.Code(err), ShouldEqual, codes.PermissionDenied)
			So(res, ShouldBeNil)
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
