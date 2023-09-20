// Copyright 2022 The LUCI Authors.
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

package bugs

import (
	"time"

	"go.chromium.org/luci/analysis/internal/analysis/metrics"
	"go.chromium.org/luci/analysis/internal/clustering"
	"go.chromium.org/luci/common/errors"
)

type BugUpdateRequest struct {
	// The bug to update.
	Bug BugID
	// Metrics for the given bug. This is only set if valid metrics are available.
	// If re-clustering is currently ongoing for the failure association rule
	// and metrics are unreliable, this will be unset to avoid erroneous
	// priority updates.
	Metrics *ClusterMetrics
	// Whether the user enabled priority updates and auto-closure for the bug.
	// If this if false, the BugUpdateRequest is only made to determine if the
	// bug is the duplicate of another bug and if the rule should be archived.
	IsManagingBug bool
	// Whether the user enabled priority updates for the Bug.
	// If this is false, the bug priority will not change.
	IsManagingBugPriority bool
	// The time when the field `IsManagingBugPriority` was last updated.
	// This is used to determine whether or not we should make priority updates by
	// if the user manually took control of the bug priority before
	// or after automatic priority updates were (re-)enabled.
	IsManagingBugPriorityLastUpdated time.Time
	// The identity of the rule associated with the bug.
	RuleID string
}

type BugUpdateResponse struct {
	// Error is the error encountered updating this bug (if any).
	// If this is set, IsDuplicate, IsAssigned, ShouldArchive and
	// DisableRulePriorityUpdates should be ignored.
	// This field is only used for errors specific to updating a particular bug.
	Error error

	// IsDuplicate is set if the bug is a duplicate of another.
	IsDuplicate bool

	// IsAssigned is set if the bug is assigned to a user.
	IsAssigned bool

	// ShouldArchive indicates if the rule for this bug should be archived.
	// This should be set if:
	// - The bug is managed by LUCI Analysis (IsManagingBug = true) and it has
	//   been marked as Closed (verified) by LUCI Analysis for the last 30
	//   days.
	// - The bug is managed by the user (IsManagingBug = false), and the
	//   bug has been closed for the last 30 days.
	ShouldArchive bool

	// DisableRulePriorityUpdates indicates that the rules `IsManagingBugUpdates`
	// field should be disabled.
	// This is set when a user manually takes control of the priority of a bug.
	DisableRulePriorityUpdates bool
}

// BugDetails holds the data of a duplicate bug, this includes it's bug id,
// and whether or not it is assigned to a user.
type BugDetails struct {
	// Bug is the source bug that was merged-into another bug.
	Bug BugID
	// IsAssigned indicated whether this bug is assigned or not.
	IsAssigned bool
}

// UpdateDuplicateSourceRequest represents a request to update the source
// bug of a duplicate bug (source bug, destination bug) pair, after
// LUCI Analysis has attempted to merge the failure association rule
// of the source to the destination.
type UpdateDuplicateSourceRequest struct {
	// BugDetails are details of the bug to be updated.
	// This includes the bug ID and whether or not it is assigned.
	BugDetails BugDetails
	// ErrorMessage is the error that occured handling the duplicate bug.
	// If this is set, it will be posted to the bug and bug will marked as
	// no longer a duplicate to avoid repeately triggering the same error.
	ErrorMessage string
	// DestinationRuleID is the rule ID for the destination bug, that the rule
	// for the source bug was merged into.
	// Only populated if Error is unset.
	DestinationRuleID string
}

var ErrCreateSimulated = errors.New("CreateNew did not create a bug as the bug manager is in simulation mode")

// BugCreateRequest captures a request to a bug filing system to
// file a new bug for a suggested cluster.
type BugCreateRequest struct {
	// The ID of the rule that is in the process of being created.
	RuleID string
	// Description is the human-readable description of the cluster.
	Description *clustering.ClusterDescription
	// Metrics contains metrics for the cluster.
	Metrics *ClusterMetrics
	// The monorail components (if any) to use.
	// This value is ignored for Buganizer.
	MonorailComponents []string
	// The buganizer component id (if any) to use.
	// This value is ignored for Monorail.
	BuganizerComponent int64
}

// ClusterMetrics captures measurements of a cluster's impact and
// other variables, as needed to control the priority and verified
// status of bugs.
type ClusterMetrics map[metrics.ID]MetricValues

// MetricValues captures measurements for one metric, over
// different timescales.
type MetricValues struct {
	OneDay   int64
	ThreeDay int64
	SevenDay int64
}
