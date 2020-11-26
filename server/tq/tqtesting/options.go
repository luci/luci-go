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

// TODO(vadimsh): Add more stop conditions when needed:
//  StopBeforeTask(taskKind)

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

// StopAfterTask will stop the scheduler after it finishes executing the task of
// matching task class ID.
func StopAfterTask(taskClassID string) RunOption {
	return stopAfterTask{taskClassID}
}

type stopAfterTask struct {
	taskClassID string
}

func (stopAfterTask) isOption() {}

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
