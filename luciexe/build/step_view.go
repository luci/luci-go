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

package build

// StepView is a window into the build State.
//
// You can obtain/manipulate this with the Step.Modify method.
type StepView struct{}

// Start will change the status of this Step from SCHEDULED to STARTED and
// initializes StartTime.
//
// This must only be called for ScheduleStep invocations. If the step is already
// started (e.g. it was produced via Step() or Start() was already called), this
// panics.
func (*StepState) Start() {
	panic("implement")
}

// Modify allows you to atomically manipulate the StepView for this StepState.
//
// Blocking in Modify will block other callers of Modify and Set*, as well as
// the ability for the build State to be sent (with the function set by
// OptSend).
//
// The Set* methods should be preferred unless you need to read/modify/write
// View items.
func (*StepState) Modify(func(*StepView)) {
	panic("implement")
}

// SetSummaryMarkdown atomically sets the step's SummaryMarkdown field.
func (*StepState) SetSummaryMarkdown(string) {
	panic("implement")
}
