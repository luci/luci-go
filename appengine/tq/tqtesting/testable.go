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

// Package tqtesting can be used in unit tests to simulate task queue calls
// produced by tq.Dispatcher.
package tqtesting

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/service/taskqueue"
)

// GetTestable returns an interface for TQ intended to be used only from tests.
//
// Panics if used with production Task Queue implementation.
func GetTestable(ctx context.Context, d *tq.Dispatcher) Testable {
	tqt := taskqueue.GetTestable(ctx)
	if tqt == nil {
		panic("not a testable task queue implementation")
	}
	internals := d.Internals().(dispatcherInternals)
	return &testableImpl{internals, tqt}
}

// Testable can be used from unit tests that posts TQ tasks.
//
// It assumes Dispatcher is in complete control of all Task Queue tasks, e.g.
// if some handlers add task queue tasks directly, they are going to be
// clobbered by Testable's PopTasks.
type Testable interface {
	// CreateQueues creates all push queues used by registered tasks.
	CreateQueues()

	// GetScheduledTasks fetches all scheduled tasks.
	//
	// Returned tasks are sorted by ETA (earliest first) and Name.
	GetScheduledTasks() TaskList

	// ExecuteTask executes a handler for the given task in a derivative of a
	// given context.
	//
	// Returns whatever the handle returns or a general error if the task can't
	// be dispatched.
	ExecuteTask(ctx context.Context, task Task, hdr *taskqueue.RequestHeaders) error

	// RunSimulation simulates task queue service by running enqueued tasks.
	//
	// It looks at all pending tasks, picks the one with smallest ETA, moves the
	// test clock and executes the task, looks at all pending tasks again, picks
	// the one with smallest ETA, and so on ...
	//
	// Panics if there's no test clock in the context. Assumes complete control
	// of the task queue service (e.g. if something is popping or resetting tasks
	// in parallel, bad things will happen).
	//
	// If it encounters an unrecognized task, calls params.UnknownTaskHandler to
	// handle it. Unrecognized tasks are still returned in 'executed' and
	// 'pending' sets, except they don't have 'Payload' set.
	//
	// It stops whenever any of the following happens:
	//   * The queue of pending tasks is empty.
	//   * ETA of the next task is past deadline (set via SimulationParams).
	//   * ShouldStopBefore(...) returns true for the next to-be-executed task.
	//   * ShouldStopAfter(...) returns true for the just-executed task.
	//   * A task returns an error. The bad task will be last in 'executed' list.
	//
	// Returns:
	//   executed: executed tasks, in order of their execution.
	//   pending: tasks to be executed (when hitting a deadline or an error).
	//   err: an error produced by the failed task (when exiting on an error).
	RunSimulation(ctx context.Context, params *SimulationParams) (executed, pending TaskList, err error)
}

// SimulationParams are passed to RunSimulation.
type SimulationParams struct {
	Deadline           time.Time                     // default is "don't stop on deadline"
	ShouldStopBefore   func(t Task) bool             // returns true if simulation should stop
	ShouldStopAfter    func(t Task) bool             // returns true if simulation should stop
	UnknownTaskHandler func(t *taskqueue.Task) error // handles unrecognized tasks
}

// Task represents a scheduled tq Task.
type Task struct {
	Task    *taskqueue.Task // original task queue task
	Payload proto.Message   // deserialized payload or nil if unrecognized
}

// dispatcherInternals is secretly exposed by Dispatcher.Internals.
//
// Implemented in tq.internalsImpl.
//
// BEWARE: There are no compile time type checks here. If you add a method or
// modify existing one make sure tq.internalsImpl is modified accordingly. Unit
// tests will fail if something is not right.
type dispatcherInternals interface {
	GetBaseURL() string
	GetAllQueues() []string
	GetPayload(blob []byte) (proto.Message, error)
	GetHandler(payload proto.Message) (cb tq.Handler, q string, err error)
	WithRequestHeaders(ctx context.Context, hdr *taskqueue.RequestHeaders) context.Context
}

////////////////////////////////////////////////////////////////////////////////

// TaskList is a sortable list of Task structs.
type TaskList []Task

func (l TaskList) Len() int           { return len(l) }
func (l TaskList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l TaskList) Less(i, j int) bool { return l[i].isLessThan(&l[j]) }

func (t *Task) isLessThan(other *Task) bool {
	switch {
	case t.Task.ETA.Before(other.Task.ETA):
		return true
	case other.Task.ETA.Before(t.Task.ETA):
		return false
	}
	return t.Task.Name < other.Task.Name
}

// Payloads collects payloads of each task into a slice.
//
// Useful when writing asserts in tests.
func (l TaskList) Payloads() (out []proto.Message) {
	out = make([]proto.Message, len(l))
	for i, t := range l {
		out[i] = t.Payload
	}
	return
}

////////////////////////////////////////////////////////////////////////////////

// testableImpl actually implements Testable.
type testableImpl struct {
	d   dispatcherInternals
	tqt taskqueue.Testable
}

func (t *testableImpl) CreateQueues() {
	for _, q := range t.d.GetAllQueues() {
		if q != "default" { // "default" queue always exists
			t.tqt.CreateQueue(q)
		}
	}
}

func (t *testableImpl) GetScheduledTasks() (out TaskList) {
	baseURL := t.d.GetBaseURL()
	for _, tasks := range t.tqt.GetScheduledTasks() {
		for _, task := range tasks {
			// Handle only tasks submitted by tq.Dispatcher.
			if strings.HasPrefix(task.Path, baseURL) {
				payload, _ := t.d.GetPayload(task.Payload)
				out = append(out, Task{
					Task:    task,
					Payload: payload,
				})
			}
		}
	}
	sort.Sort(out)
	return
}

func (t *testableImpl) ExecuteTask(ctx context.Context, task Task, hdr *taskqueue.RequestHeaders) error {
	if task.Payload == nil {
		return fmt.Errorf("can't execute a task without payload, not a tq task?")
	}

	cb, q, err := t.d.GetHandler(task.Payload)
	if err != nil {
		return err
	}

	headers := taskqueue.RequestHeaders{}
	if hdr != nil {
		headers = *hdr
	}
	headers.QueueName = q
	headers.TaskName = task.Task.Name
	headers.TaskETA = task.Task.ETA

	return cb(t.d.WithRequestHeaders(ctx, &headers), task.Payload)
}

////////////////////////////////////////////////////////////////////////////////

func (t *testableImpl) RunSimulation(ctx context.Context, params *SimulationParams) (executed, pending TaskList, err error) {
	tc := clock.Get(ctx).(testclock.TestClock)

	var deadline time.Time
	var shouldStopBefore func(t Task) bool
	var shouldStopAfter func(t Task) bool
	var unknownHandler func(t *taskqueue.Task) error
	if params != nil {
		deadline = params.Deadline
		shouldStopBefore = params.ShouldStopBefore
		shouldStopAfter = params.ShouldStopAfter
		unknownHandler = params.UnknownTaskHandler
	}

loop:
	for {
		earliest, queue := t.pickEarliestETA()
		switch {
		case earliest == nil:
			break loop // no more tasks
		case !deadline.IsZero() && earliest.Task.ETA.After(deadline):
			break loop // deadline reached
		case shouldStopBefore != nil && shouldStopBefore(*earliest):
			break loop // stop condition reached
		}

		if err = taskqueue.Delete(ctx, queue, earliest.Task); err != nil {
			panic("impossible, the task must be in the queue")
		}
		executed = append(executed, *earliest)
		tc.Set(earliest.Task.ETA)

		if earliest.Payload == nil {
			if unknownHandler != nil {
				err = unknownHandler(earliest.Task)
			} else {
				err = fmt.Errorf("unrecognized TQ task for handler at %s", earliest.Task.Path)
			}
		} else {
			err = t.ExecuteTask(ctx, *earliest, nil)
		}

		if err != nil {
			break
		}
		if shouldStopAfter != nil && shouldStopAfter(*earliest) {
			break
		}
	}

	pending = t.GetScheduledTasks()
	return
}

func (t *testableImpl) pickEarliestETA() (earliest *Task, queue string) {
	// TODO(vadimsh): This is horribly inefficient in case there are large number
	// of pending tasks. If it becomes an issue, taskqueue's Testable interface
	// should be modified to return earliest task directly (it can pick it more
	// efficiently, since it stores tasks in sorted priority queues already).
	for q, tasks := range t.tqt.GetScheduledTasks() {
		for _, task := range tasks {
			payload, _ := t.d.GetPayload(task.Payload)
			tt := Task{
				Task:    task,
				Payload: payload,
			}
			if earliest == nil || tt.isLessThan(earliest) {
				earliest = &tt
				queue = q
			}
		}
	}
	return
}
