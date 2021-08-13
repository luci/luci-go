// Copyright 2020 The LUCI Authors.
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

package tqtesting

// RunOption influences behavior of Run call.
type RunOption interface {
	isOption()
}

// StopWhenDrained will stop the scheduler after it finishes executing the
// last task and there are no more tasks scheduled.
//
// It is naturally racy if there are other goroutines that submit tasks
// concurrently. In this situation there may be a pending queue of tasks even
// if Run stops.
func StopWhenDrained() RunOption {
	return stopWhenDrained{}
}

type stopWhenDrained struct{}

func (stopWhenDrained) isOption() {}

// StopAfterTask will stop the scheduler after it finishes executing a task of
// the given task class ID.
func StopAfterTask(taskClassID string) RunOption {
	return StopAfter(func(t *Task) bool { return t.Class == taskClassID })
}

// StopAfter will stop the scheduler if the given function returns true,
// given the just finished task.
func StopAfter(f func(t *Task) bool) RunOption {
	return stopAfter{f}
}

type stopAfter struct {
	examine func(t *Task) bool
}

func (stopAfter) isOption() {}

// StopBeforeTask will stop the scheduler if the next task to be executed has
// the given task class ID.
//
// The same caveats of StopBefore also apply for StopBeforeTask.
func StopBeforeTask(taskClassID string) RunOption {
	return StopBefore(func(t *Task) bool { return t.Class == taskClassID })
}

// StopBefore will stop the scheduler if the given function returns true,
// given the next task to be executed.
//
// If such next task has specified ETA, StopBeforeTask does NOT provide any
// guarantee about what `clock.Now` returns by the time Run stops.
//
// It is naturally racy if there are other goroutines that submit tasks
// concurrently. In this situation there may be a different next task (by ETA)
// when Run stops.
func StopBefore(f func(t *Task) bool) RunOption {
	return stopBefore{f}
}

type stopBefore struct {
	examine func(t *Task) bool
}

func (stopBefore) isOption() {}

// ParallelExecute instructs the scheduler to call executor's Execute method
// in a separate goroutine instead of serially in Run.
//
// This more closely resembles real-life behavior but may introduce more
// unpredictability into tests due to races.
func ParallelExecute() RunOption {
	return parallelExecute{}
}

type parallelExecute struct{}

func (parallelExecute) isOption() {}

// SkipPubSubTask instructs the schedule to skip executing PubSub tasks.
func SkipPubSubTask() RunOption {
	return skipPubSubTask{}
}

type skipPubSubTask struct{}

func (skipPubSubTask) isOption() {}
