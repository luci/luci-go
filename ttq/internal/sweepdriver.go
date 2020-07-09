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

package internal

import "context"

// SweepDriver drives the sweeping process.
//
// SweepDriver owns a Sweepable object.
// SweepDriver should consist of:
//  * cron to to call Sweepable.Sweep frequently.
//  * task queue to execute SweepWorkItem asynchroneusly.
type SweepDriver interface {
	// AsyncSweep schedules a SweepWorkItem for later execution.
	//
	// If dedupeKey is given, work items with the same value should be
	// deduplicated. Semantics is the same as in named tasks of Cloud Tasks.
	// dedupeKey values are guaranteed to be well-distributed.
	AsyncSweep(ctx context.Context, w *SweepWorkItem, dedupeKey string) error
}

// Sweepable abstracts out the TTQ implementation's public interface for
// sweeping.
type Sweepable interface {
	// SweepCron must be called frequently to initiate sweeping.
	SweepCron(context.Context) error
	// ExecSweepWorkItem executes a SweepWorkItem scheduled via SweepDriver.AsyncSweep.
	ExecSweepWorkItem(context.Context, *SweepWorkItem) error
}

// SweepDriverFactory creates a SweepDriver given an object to sweep.
type SweepDriverFactory func(Sweepable) SweepDriver
