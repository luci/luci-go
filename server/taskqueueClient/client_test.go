// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package taskqueueClient

import (
	"net/http"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/errors"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func cloneTask(t *Task) *Task {
	if t == nil {
		return nil
	}

	c := *t
	c.Payload = make([]byte, len(t.Payload))
	copy(c.Payload, t.Payload)
	return &c
}

// testTQ is an implementation of the taskQueue interface for testing.
type testTQ struct {
	sync.Mutex
	queueName string

	err   error
	tasks map[string]*Task
}

func (tq *testTQ) addTasks(c context.Context, tasks ...string) {
	tq.Lock()
	defer tq.Unlock()

	if tq.tasks == nil {
		tq.tasks = map[string]*Task{}
	}

	for _, s := range tasks {
		tq.tasks[s] = &Task{
			ID:          s,
			Queue:       tq.queueName,
			EnqueueTime: clock.Now(c),
			Payload:     []byte(s),
		}
	}
}

func (tq *testTQ) setError(err error) {
	tq.Lock()
	defer tq.Unlock()
	tq.err = err
}

func (tq *testTQ) currentTasks() []string {
	tq.Lock()
	defer tq.Unlock()

	tasks := make([]string, 0, len(tq.tasks))
	for k := range tq.tasks {
		tasks = append(tasks, k)
	}
	sort.Strings(tasks)
	return tasks
}

func (tq *testTQ) lease(c context.Context, tasks int, d time.Duration) ([]*Task, error) {
	tq.Lock()
	defer tq.Unlock()

	if tq.err != nil {
		return nil, tq.err
	}

	taskNames := make([]string, 0, len(tq.tasks))
	for t := range tq.tasks {
		taskNames = append(taskNames, t)
	}
	sort.Strings(taskNames)

	ts := make([]*Task, 0, len(taskNames))
	now := clock.Now(c)
	for _, taskName := range taskNames {
		if len(ts) >= tasks {
			break
		}

		task := tq.tasks[taskName]
		if !(task.LeaseExpireTime.IsZero() || now.After(task.LeaseExpireTime)) {
			// Task is leased.
			continue
		}
		task.LeaseExpireTime = now.Add(d)
		ts = append(ts, cloneTask(task))
	}
	return ts, nil
}

func (tq *testTQ) deleteTask(c context.Context, task string) error {
	tq.Lock()
	defer tq.Unlock()

	if tq.err != nil {
		return tq.err
	}
	if _, ok := tq.tasks[task]; !ok {
		return errors.New("task does not exist")
	}
	delete(tq.tasks, task)
	return nil
}

func (tq *testTQ) updateLease(c context.Context, task string, d time.Duration) (*Task, error) {
	tq.Lock()
	defer tq.Unlock()

	if tq.err != nil {
		return nil, tq.err
	}

	t := tq.tasks[task]
	if t == nil {
		return nil, errors.New("task does not exist")
	}
	if t.LeaseExpireTime.IsZero() || t.LeaseExpireTime.Before(clock.Now(c)) {
		return nil, errors.New("task is not leased")
	}

	t.LeaseExpireTime = clock.Now(c).Add(d)
	return cloneTask(t), nil
}

func TestClient(t *testing.T) {
	t.Parallel()

	const halfLease = DefaultLease / 2

	Convey(`A testing configuration`, t, func() {
		c, tc := testclock.UseTime(context.Background(), testclock.TestTimeLocal)
		c, cancelFunc := context.WithCancel(c)
		defer cancelFunc()

		tq := testTQ{
			queueName: "testQ",
		}
		o := Options{
			Client: &http.Client{}, // Will not be used b/c of testingParams.
			Queue:  tq.queueName,
			Tasks:  4,

			t: &testingParams{
				tq: &tq,
			},
		}

		Convey(`RunTasks will panic if no Client is configured.`, func() {
			o.Client = nil
			So(func() { RunTasks(c, o, nil) }, ShouldPanic)
		})

		Convey(`A Client will lease, process, and delete.`, func() {
			tasks := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}
			tq.addTasks(c, tasks...)

			executedC := make(chan string)
			doneC := make(chan struct{})
			go func() {
				defer close(doneC)
				RunTasks(c, o, func(ic context.Context, t Task) bool {
					executedC <- t.ID
					return true
				})
			}()

			var executed []string
			for i := 0; i < len(tasks); i++ {
				executed = append(executed, <-executedC)
			}

			// Wait for all tasks to be processed.
			cancelFunc()
			<-doneC

			sort.Strings(executed)
			So(executed, ShouldResemble, tasks)
			So(tq.currentTasks(), ShouldHaveLength, 0)
		})

		Convey(`A Client will continue polling for new tasks.`, func() {
			// Control each "sleep" call that happens between lease retrieval.
			timeC := make(chan time.Duration, 1)
			tc.SetTimerCallback(func(od time.Duration, t clock.Timer) {
				d, ok := <-timeC
				if !ok {
					return
				}
				tc.Add(d)
			})

			taskC := make(chan string)
			doneC := make(chan struct{})
			go func() {
				defer close(doneC)
				RunTasks(c, o, func(ic context.Context, t Task) bool {
					taskC <- t.ID
					return true
				})
			}()

			// First 5 rounds: no tasks.
			for i := 0; i < 5; i++ {
				timeC <- DefaultNoTasksSleep
			}
			close(timeC)

			// Now add some tasks and wait for our lease polling to pick them up.
			tq.addTasks(c, "a", "b", "c")

			var tasks []string
			for i := 0; i < 3; i++ {
				tasks = append(tasks, <-taskC)
			}
			cancelFunc()
			<-doneC

			sort.Strings(tasks)
			So(tasks, ShouldResemble, []string{"a", "b", "c"})
			So(tq.currentTasks(), ShouldHaveLength, 0)
		})

		Convey(`If a task panics, it will be caught.`, func() {
			tq.addTasks(c, "a")

			So(func() {
				doneC := make(chan struct{})
				taskExecutedC := make(chan struct{})
				go func() {
					defer close(doneC)
					RunTasks(c, o, func(ic context.Context, t Task) bool {
						defer close(taskExecutedC)
						panic("test error")
					})
				}()

				<-taskExecutedC
				cancelFunc()
				<-doneC
			}, ShouldNotPanic)

			// The task should have its lease set to zero.
			So(tq.tasks["a"].LeaseExpireTime, ShouldResemble, clock.Now(c))
		})

		Convey(`If a task takes too long, it will have its lease refreshed indefinitely.`, func() {
			tq.addTasks(c, "a")

			o.t.leaseUpdateC = make(chan *Task)

			doneC := make(chan struct{})
			taskExecutedC := make(chan int)
			go func() {
				defer close(doneC)
				RunTasks(c, o, func(ic context.Context, t Task) bool {
					updates := 0
					defer func() {
						taskExecutedC <- updates
					}()

					// Wait for 20 lease updates. Each time, immediately expire the next
					// lease.
					for ; updates < 20; updates++ {
						t := <-o.t.leaseUpdateC
						tc.Set(t.LeaseExpireTime.Add(-halfLease))
					}
					<-o.t.leaseUpdateC
					return true
				})
			}()

			updates := <-taskExecutedC
			cancelFunc()
			<-doneC

			So(updates, ShouldEqual, 20)
			So(tq.currentTasks(), ShouldHaveLength, 0)
		})

		Convey(`If a task expires during execution, it will have its Context cancelled.`, func() {
			tq.addTasks(c, "a")

			o.t.leaseUpdateC = make(chan *Task)
			o.t.leaseUpdateFailedC = make(chan *Task)

			errC := make(chan error)
			doneC := make(chan struct{})
			go func() {
				defer close(doneC)

				RunTasks(c, o, func(ic context.Context, t Task) bool {
					// Our task queue will return errors for lease refresh.
					tq.setError(errors.New("test error"))

					// Advance our clock past the refresh threshold, but not past our
					// lease expiration threshold. A lease refresh will be attempted, and
					// will fail (b/c of the above error).
					<-o.t.leaseUpdateC
					tc.Set(t.LeaseExpireTime.Add(-halfLease))
					<-o.t.leaseUpdateFailedC

					// Advance our clock past the expiration threshold.
					tc.Set(t.LeaseExpireTime)

					// This should now be unblocked.
					<-ic.Done()

					tq.setError(nil)
					errC <- ic.Err()
					return true
				})
			}()

			So(<-errC, ShouldEqual, context.Canceled)
			cancelFunc()
			<-doneC

			So(tq.currentTasks(), ShouldResemble, []string{"a"})
		})
		Convey(`If the root Context is canceled, task dispatch will shut down.`, func() {
			o.Tasks = 4
			tq.addTasks(c, "a", "b", "c", "d")

			c, cancelFunc := context.WithCancel(c)
			waitingC := make(chan string)
			finishedC := make(chan string)
			seenC := make(chan []string)
			go func() {
				var reaped []string
				defer func() {
					seenC <- reaped
					close(seenC)
				}()

				// Wait for all 4 tasks to be running, then cancel our Context.
				for i := 0; i < 4; i++ {
					<-waitingC
				}

				cancelFunc()

				// Add additional tasks. None of these should be executed, since we've
				// been canceled.
				tq.addTasks(c, "e", "f")

				// Collect the names of the tasks that have executed.
				for t := range finishedC {
					reaped = append(reaped, t)
				}
			}()

			RunTasks(c, o, func(ic context.Context, t Task) bool {
				waitingC <- t.ID
				<-ic.Done()
				finishedC <- t.ID
				return true
			})
			close(finishedC)

			// Confirm that all of our tasks have been executed.
			seen := <-seenC
			sort.Strings(seen)
			So(seen, ShouldResemble, []string{"a", "b", "c", "d"})
		})
	})
}
