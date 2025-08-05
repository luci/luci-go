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
	bugspb "go.chromium.org/luci/analysis/internal/bugs/proto"
	"go.chromium.org/luci/analysis/internal/clustering"
)

type BugUpdateRequest struct {
	// The bug to update.
	Bug BugID
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
	// The bug management state of the rule. Only set if policy-based bug
	// management should be used; see Metrics field otherwise.
	BugManagementState *bugspb.BugManagementState
	// Information on whether a bug closure should be invalidated based on if the
	// metrics met the thresholds for the one day, three days, or seven days after
	// the bug closure.
	InvalidationStatus BugClosureInvalidationStatus
}

type BugUpdateResponse struct {
	// Error is the error encountered updating this specific bug (if any).
	//
	// Even if an error is set here, partial success is possible and
	// actions may have been taken on the bug which require corresponding
	// rule updates. As such, even if an error is set here, the consumer
	// of this response must action the rule updates specified by
	// DisableRulePriorityUpdates, RuleAssociationNotified and
	// PolicyActivationsNotified to avoid us unnecessarily repeating
	// these actions on the bug the next time bug management runs.
	//
	// Similarly, even when an error occurs, the producer of this response
	// still has an an obligation to set DisableRulePriorityUpdates,
	// RuleAssociationNotified and PolicyActivationsNotified correctly.
	// In case of complete failures, leaving DisableRulePriorityUpdates
	// and PolicyActivationsNotified set to false, and PolicyActivationsNotified
	// empty will satisfy this requirement.
	Error error

	// IsDuplicate is set if the bug is a duplicate of another.
	// Set to true to trigger updating/merging failure association rules.
	//
	// If we could not fetch the bug for some reason and cannot determine
	// if the bug should be archived, this must be set to false.
	IsDuplicate bool

	// IsDuplicateAndAssigned is set if the bug is assigned to a user.
	// Only set if IsDuplicate is set.
	IsDuplicateAndAssigned bool

	// ShouldArchive indicates if the rule for this bug should be archived.
	// This should be set if:
	// - The bug is managed by LUCI Analysis (IsManagingBug = true) and it has
	//   been marked as Closed (verified) by LUCI Analysis for the last 30
	//   days.
	// - The bug is managed by the user (IsManagingBug = false), and the
	//   bug has been closed for the last 30 days.
	//
	// If we could not fetch the bug for some reason and cannot determine
	// if the bug should be archived, this must be set to false.
	ShouldArchive bool

	// DisableRulePriorityUpdates indicates that the rules `IsManagingBugUpdates`
	// field should be disabled.
	// This is set when a user manually takes control of the priority of a bug.
	//
	// If we could not fetch the bug for some reason and cannot determine
	// if the user has taken control of the bug, this must be set to false.
	DisableRulePriorityUpdates bool

	// Whether the association between the rule and the bug was notified.
	// Set to true to set BugManagementState.RuleAssociationNotified.
	RuleAssociationNotified bool

	// The set of policy IDs for which activation was notified.
	// This may differ from the set which is pending notification in the case of
	// partial or total failure to update the bug.
	// If a policy ID is included here,
	// BugManagementState.Policies[<policyID>].ActivationNotified will be
	// set to true on the rule.
	PolicyActivationsNotified map[PolicyID]struct{}
	// The result of validating a bug closure. This is only applicable to closed
	// bugs.
	BugClosureValidationResult BugClosureInvalidationResult
	// Title of the issue that was updated.
	BugTitle string
}

// DuplicateBugDetails holds the data of a duplicate bug, this includes it's bug id,
// and whether or not it is assigned to a user.
type DuplicateBugDetails struct {
	// The rule ID.
	RuleID string
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
	BugDetails DuplicateBugDetails
	// ErrorMessage is the error that occured handling the duplicate bug.
	// If this is set, it will be posted to the bug and bug will marked as
	// no longer a duplicate to avoid repeately triggering the same error.
	ErrorMessage string
	// DestinationRuleID is the rule ID for the destination bug, that the rule
	// for the source bug was merged into.
	// Only populated if Error is unset.
	DestinationRuleID string
}

// BugCreateRequest captures a request to a bug filing system to
// file a new bug for a suggested cluster.
type BugCreateRequest struct {
	// The ID of the rule that is in the process of being created.
	RuleID string
	// The rule predicate, defining which failures are being associated.
	RuleDefinition string
	// Description is the human-readable description of the cluster.
	Description *clustering.ClusterDescription
	// The monorail components (if any) to use.
	// This value is ignored for Buganizer.
	MonorailComponents []string
	// The buganizer component id (if any) to use.
	// This value is ignored for Monorail.
	BuganizerComponent int64
	// Automatic bug-management state for a single failure association rule.
	BugManagementState *bugspb.BugManagementState
	// Whether a new bug is being created for an existing rule
	// (e.g. in response to invalidating the fix for an existing bug),
	// rather than filing an entirely new rule.
	ReuseRule bool
}

type BugCreateResponse struct {
	// The error encountered creating the bug. Note that this error
	// may have occured after a partial success; for example, we may have
	// successfully created a bug but failed on a subequent RPC to
	// posting comments for the policies that have activated.
	//
	// In the case of partial success, ID shall still be set to allow
	// creation of the rule. This avoids duplicate bugs being created
	// from repeated failed attempts to completely create a bug.
	// The policies for which comments could not be posted will
	// not be set to true under BugManagementState.Policies[<policyID>].ActivationNotified
	// so that these are retried as part of the bug update process.
	Error error

	// Simulated indicates whether the bug creation was simulated,
	// i.e. did not actually occur. If this is set, the returned
	// bug ID should be ignored.
	Simulated bool

	// The system-specific bug ID for the bug that was created.
	// See BugID.ID.
	// If we failed to create a bug, set to "" (empty).
	ID string

	// The set of policy IDs for which activation was successfully notified.
	// This may differ from the list in the request in the case of
	// partial or total failure to create the bug.
	// If a policy ID is included here,
	// BugManagementState.Policies[<policyID>].ActivationNotified will be
	// set to true on the rule.
	PolicyActivationsNotified map[PolicyID]struct{}
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
