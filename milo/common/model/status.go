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

//go:generate stringer -type=Status,BotStatus

package model

import "encoding/json"

// Status indicates the status of some piece of the CI; builds, steps, builders,
// etc. The UI maps this to a color, and some statuses may map to the same
// color; however, we attempt to preserve full informational fidelity.
type Status int

const (
	// NotRun if the component has not yet been run.  E.g. if the component has
	// been scheduled, but is pending execution.
	NotRun Status = iota

	// Running if the component is currently running.
	Running

	// Success if the component has finished executing and accomplished what it
	// was supposed to.
	Success

	// Failure if the component has finished executing and failed in
	// a non-exceptional way. e.g. if a test completed execution, but determined
	// that the code was bad.
	Failure

	// Warning if the component has finished executing, but encountered
	// non-stoppage problems. e.g. if a test completed execution, but determined
	// that the code was slow (but not slow enough to be a failure).
	Warning

	// InfraFailure if the component has finished incompletely due to an
	// infrastructure layer.
	//
	// This is used to categorize all unknown errors.
	InfraFailure

	// Exception if the component has finished incompletely due to an exceptional
	// error in the task. That means the infrastructure layers executed the task
	// completely, but the task self-reported that it failed in an exceptional
	// way.
	//
	// DON'T USE THIS IN ANY NEW CODE. Instead, prefer InfraFailure.
	Exception

	// Expired if the component was never scheduled due to resource exhaustion.
	Expired

	// Cancelled if the component had external intervention to stop it after it
	// was scheduled, but before it completed on its own.
	Cancelled
)

// Terminal returns true if the step status won't change.
func (s Status) Terminal() bool {
	switch s {
	case Success, Failure, InfraFailure, Warning, Expired, Exception, Cancelled:
		return true
	default:
		return false
	}
}

// MarshalJSON renders enums into String rather than an int when marshalling.
func (s Status) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

// BotStatus indicates the status of a machine.
type BotStatus int

const (
	// Idle means the bot is ready to accept a job.
	Idle BotStatus = iota
	// Busy means the bot is currently running a job, or recently finished
	// a job and may not be ready to accept a new job yet.
	Busy
	// Offline means the bot is neither ready to accept a job nor
	// currently running a job.  The bot could be dead or quarantined.
	Offline
)
