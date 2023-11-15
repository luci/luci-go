// Copyright 2021 The LUCI Authors.
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

// Package itriager defines interface of a CL component triage process.
//
// It's a separate package to avoid circular imports.
package itriager

import (
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/run/runcreator"
)

// Triage triages a component deciding what and when has to be done.
//
// Triage is called outside of transaction context.
// Triage may be retried with the same or different input, possibly
// concurrently, on the same component.
//
// Triage must treat all given data as read-only. If it needs to modify
// component, it should use copy-on-write approach.
//
// Project Manager guarantees that for each CL of the given component:
//   - there is a PCL via Supporter interface,
//   - and for each such CL's dependency, either:
//   - dep is not yet loaded,
//   - OR dep must be itself a component's CL.
//
// May return special, possibly wrapped, ErrOutdatedPMState to signal that
// Triage function has detected outdated PMState.
type Triage func(ctx context.Context, c *prjpb.Component, s PMState) (Result, error)

// Result is the result of a component traige.
//
// Consistency notes:
//
// The RunsToCreate are processed first, independently from each other.
// If at least one fails, then NewValue and CLsTopurge are not saved.
// This ensures that in case of Run creation failure, the next PM invocation
// will call Triage again.
//
// If RunsToCreate aren't specified OR all of them are created successfully,
// then the CLsTopurge and NewValue are saved atomically.
type Result struct {
	// RunsToCreate is set if a Run has to be created.
	//
	// If set, must contain fully populated runcreator.Creator objects.
	RunsToCreate []*runcreator.Creator

	// TODO(tandrii): support cancel Run actions.

	// NewValue specifies the new value of a component if non-nil.
	//
	// Must be set if the input component is .TriageRequired.
	// If not set, implies that component doesn't have to be changed.
	NewValue *prjpb.Component

	// CLsToPurge must contain PurgeCLTasks tasks with with these (sub)fields set:
	//  * .PurgingCL.Clid
	//  * .PurgeReasons
	CLsToPurge []*prjpb.PurgeCLTask
	// CLsToTriggerDeps are CLs to propagate the CQ votes to the deps of.
	CLsToTriggerDeps []*prjpb.TriggeringCLDeps
}

// PMState provides limited access to resources of Project Manager (PM) state for
// ease of testing and correctness.
//
// All data exposed by the PMState must not be modified and remains immutable
// during the lifetime of a Triage() function execution.
type PMState interface {
	// PCL provides access to the CLs tracked by the PM.
	//
	// Returns nil if clid refers to a CL not known to the PM.
	PCL(clid int64) *prjpb.PCL

	// PurgingCL provides access to CLs being purged on behalf of the PM.
	//
	// Returns nil if given CL isn't being purged.
	PurgingCL(clid int64) *prjpb.PurgingCL

	// ConfigGroup returns a ConfigGroup for a given index of the current
	// (from the view point of PM) LUCI project config version.
	ConfigGroup(index int32) *prjcfg.ConfigGroup

	// TriggeringCLDeps provides access to CLs being triggered.
	//
	// Returns nil if given CL isn't being triggered.
	TriggeringCLDeps(clid int64) *prjpb.TriggeringCLDeps
}

// ErrOutdatedPMState signals that PMState is out dated.
var ErrOutdatedPMState = errors.New("outdated PM state")

// IsErrOutdatedPMState returns true if given error is a possibly wrapped
// ErrOutdatedPMState.
func IsErrOutdatedPMState(err error) bool {
	return errors.Contains(err, ErrOutdatedPMState)
}
