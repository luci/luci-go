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

import (
	"context"
)

// StepState represents the state of a single step.
//
// This is properly initialized by the Step and ScheduleStep functions.
type StepState struct{}

// Step adds a new step to the build.
//
// The step will have a "RUNNING" status with a StartTime.
//
// The returned context is updated so that calling Step/ScheduleStep on it will create sub-steps.
//
// If `name` contains `|` this function will panic, since this is a reserved
// character for delimiting hierarchy in steps.
//
// Duplicate step names will be disambiguated by appending " (N)" for the 2nd,
// 3rd, etc. duplicate.
//
// The returned context will have `name` embedded in it; Calling Step or
// ScheduleStep with this context will generate a sub-step.
//
// You MUST call StepState.End. To automatically map errors and panics to their
// correct visual representation, End the Step like:
//
//    var err error
//    step, ctx := build.Step(ctx, "Step name")
//    defer func() { step.End(err) }()
//
//    err = opThatErrsOrPanics(ctx)
//
// NOTE: A panic will still crash the program as usual. This does NOT
// `recover()` the panic. Please use conventional Go error handling and control
// flow mechanisms.
func Step(ctx context.Context, name string) (*StepState, context.Context) {
	panic("not implemented")
}

// ScheduleStep is like Step, except that it leaves the new step in the
// SCHEDULED status, and does not set a StartTime.
//
// The step will move to RUNNING when calling any other methods on
// the StepState, when creating a sub-Step, or if you explicitly call
// StepState.Start().
func ScheduleStep(ctx context.Context, name string) (*StepState, context.Context) {
	panic("not implemented")
}

// End sets the step's final status, according to `err` (See ExtractStatus).
//
// End will also be able to set INFRA_FAILURE status and log additional
// information if the program is panic'ing.
//
// End must be invoked like:
//
//    var err error
//    step, ctx := build.Step(ctx, ...)  // or build.ScheduleStep
//    defer func() { step.End(err) }()
//
//    err = opThatErrsOrPanics()
//
// NOTE: A panic will still crash the program as usual. This does NOT
// `recover()` the panic. Please use conventional Go error handling and control
// flow mechanisms.
func (*StepState) End(err error) {
	panic("not implemented")
}
