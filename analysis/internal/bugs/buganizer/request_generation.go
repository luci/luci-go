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

package buganizer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"go.chromium.org/luci/analysis/internal/bugs"
	"go.chromium.org/luci/analysis/internal/clustering"
	configpb "go.chromium.org/luci/analysis/proto/config"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/third_party/google.golang.org/genproto/googleapis/devtools/issuetracker/v1"
)

// The status which are consider to be closed.
var ClosedStatuses = map[issuetracker.Issue_Status]struct{}{
	issuetracker.Issue_FIXED:             {},
	issuetracker.Issue_VERIFIED:          {},
	issuetracker.Issue_NOT_REPRODUCIBLE:  {},
	issuetracker.Issue_INFEASIBLE:        {},
	issuetracker.Issue_INTENDED_BEHAVIOR: {},
}

// This maps the configpb priorities to issuetracker package priorities.
var configPriorityToIssueTrackerPriority = map[configpb.BuganizerPriority]issuetracker.Issue_Priority{
	configpb.BuganizerPriority_P0: issuetracker.Issue_P0,
	configpb.BuganizerPriority_P1: issuetracker.Issue_P1,
	configpb.BuganizerPriority_P2: issuetracker.Issue_P2,
	configpb.BuganizerPriority_P3: issuetracker.Issue_P3,
	configpb.BuganizerPriority_P4: issuetracker.Issue_P4,
}

// This is the name of the Priority field in IssueState.
// We use this to look for updates to issue priorities.
const priorityField = "priority"

// The result of a MakeUpdate request.
type MakeUpdateResult struct {
	// The generated request.
	request *issuetracker.ModifyIssueRequest
	// disablePriorityUpdates is set when the user has manually
	// made a priority update since the last time automatic
	// priority updates were enabled
	disablePriorityUpdates bool
}

// RequestGenerator generates new bugs or prepares existing ones
// for updates.
type RequestGenerator struct {
	// The issuetracker client that will be used to make RPCs to Buganizer.
	client Client
	// The GAE app id, e.g. "luci-analysis".
	appID string
	// The LUCI project for which we are generating bug updates. This
	// is distinct from the Buganizer project.
	project string
	// The Buganizer config of the LUCI project config.
	buganizerCfg *configpb.BuganizerProject
	// The threshold at which bugs are filed. Used here as the threshold
	// at which to re-open verified bugs.
	bugFilingThreshold *configpb.ImpactThreshold
}

// Initializes and returns a new buganizer request generator.
func NewRequestGenerator(
	client Client,
	appID, project string,
	projectCfg *configpb.ProjectConfig) *RequestGenerator {
	return &RequestGenerator{
		client:             client,
		appID:              appID,
		project:            project,
		buganizerCfg:       projectCfg.Buganizer,
		bugFilingThreshold: projectCfg.BugFilingThreshold,
	}
}

// PrepareNew generates a CreateIssueRequest for a new issue.
// It sets the default values on the bug.
func (rg *RequestGenerator) PrepareNew(impact *bugs.ClusterImpact,
	description *clustering.ClusterDescription,
	componentId int64) *issuetracker.CreateIssueRequest {

	issue := &issuetracker.Issue{
		IssueState: &issuetracker.IssueState{
			ComponentId: componentId,
			Type:        issuetracker.Issue_BUG,
			Status:      issuetracker.Issue_NEW,
			Priority:    rg.clusterPriority(impact),
			Severity:    issuetracker.Issue_S2,
			Title:       bugs.GenerateBugSummary(description.Title),
		},
		IssueComment: &issuetracker.IssueComment{
			Comment: bugs.GenerateInitialIssueDescription(description, rg.appID),
		},
	}

	return &issuetracker.CreateIssueRequest{
		Issue: issue,
	}
}

// clusterPriority returns the desired priority of the bug, if no hysteresis
// is applied.
func (rg *RequestGenerator) clusterPriority(impact *bugs.ClusterImpact) issuetracker.Issue_Priority {
	return rg.clusterPriorityWithInflatedThresholds(impact, 0)
}

// clusterPriorityWithInflatedThresholds returns the desired priority of the bug,
// if thresholds are inflated or deflated with the given percentage.
//
// See bugs.InflateThreshold for the interpretation of inflationPercent.
func (rg *RequestGenerator) clusterPriorityWithInflatedThresholds(impact *bugs.ClusterImpact, inflationPercent int64) issuetracker.Issue_Priority {
	mappings := rg.buganizerCfg.PriorityMappings
	// Default to using the lowest priorityMapping.
	priorityMapping := mappings[len(mappings)-1]
	for i := len(mappings) - 2; i >= 0; i-- {
		p := mappings[i]
		adjustedThreshold := bugs.InflateThreshold(p.Threshold, inflationPercent)
		if !impact.MeetsThreshold(adjustedThreshold) {
			// A cluster cannot reach a higher priority unless it has
			// met the thresholds for all lower priorities.
			break
		}
		priorityMapping = p
	}
	return configPriorityToIssueTrackerPriority[priorityMapping.Priority]
}

// linkToRuleComment returns a comment that links the user to the failure
// association rule in LUCI Analysis.
//
// issueId is the Buganizer issueId.
func (rg *RequestGenerator) linkToRuleComment(issueId int64) string {
	issueLink := fmt.Sprintf("https://%s.appspot.com/b/%d", rg.appID, issueId)
	return fmt.Sprintf(bugs.LinkTemplate, issueLink)
}

// PrepareLinkComment prepares a request that adds links to LUCI Analysis to
// a Buganizer bug.
func (rg *RequestGenerator) PrepareLinkComment(issueId int64) *issuetracker.CreateIssueCommentRequest {
	return &issuetracker.CreateIssueCommentRequest{
		IssueId: issueId,
		Comment: &issuetracker.IssueComment{
			Comment: rg.linkToRuleComment(issueId),
		},
	}
}

// PrepareLinkIssueCommentUpdate prepares a request that adds links to LUCI Analysis to
// a Buganizer bug by updating the issue description.
func (rg *RequestGenerator) PrepareLinkIssueCommentUpdate(issue *issuetracker.Issue) *issuetracker.UpdateIssueCommentRequest {
	linkComment := issue.IssueComment.Comment + "\n" + rg.linkToRuleComment(issue.IssueId)
	return &issuetracker.UpdateIssueCommentRequest{
		IssueId:       issue.IssueId,
		CommentNumber: 1,
		Comment: &issuetracker.IssueComment{
			Comment: linkComment,
		},
	}
}

// noPermissionComment returns a comment that explains why a bug was filed in
// the fallback component incorrectly.
//
// issueId is the Buganizer issueId.
func (rg *RequestGenerator) noPermissionComment(componentID int64) string {
	return fmt.Sprintf(bugs.NoPermissionTemplate, componentID)
}

// PrepareNoPermissionComment prepares a request that adds links to LUCI Analysis to
// a Buganizer bug.
func (rg *RequestGenerator) PrepareNoPermissionComment(issueID, componentID int64) *issuetracker.CreateIssueCommentRequest {
	return &issuetracker.CreateIssueCommentRequest{
		IssueId: issueID,
		Comment: &issuetracker.IssueComment{
			Comment: rg.noPermissionComment(componentID),
		},
	}
}

// UpdateDuplicateSource updates the source bug of a (source, destination)
// duplicate bug pair, after LUCI Analysis has attempted to merge their
// failure association rules.
func (rg *RequestGenerator) UpdateDuplicateSource(issueId int64, errorMessage, destinationRuleID string, isAssigned bool) *issuetracker.ModifyIssueRequest {
	updateRequest := &issuetracker.ModifyIssueRequest{
		IssueId: issueId,
		AddMask: &fieldmaskpb.FieldMask{
			Paths: []string{},
		},
		Add: &issuetracker.IssueState{},
		RemoveMask: &fieldmaskpb.FieldMask{
			Paths: []string{},
		},
		Remove: &issuetracker.IssueState{},
	}
	if errorMessage != "" {
		if isAssigned {
			updateRequest.Add.Status = issuetracker.Issue_ASSIGNED
		} else {
			updateRequest.Add.Status = issuetracker.Issue_NEW
		}
		updateRequest.AddMask.Paths = append(updateRequest.AddMask.Paths, "status")
		updateRequest.IssueComment = &issuetracker.IssueComment{
			Comment: strings.Join([]string{errorMessage, rg.linkToRuleComment(issueId)}, "\n\n"),
		}
	} else {
		bugLink := fmt.Sprintf("https://%s.appspot.com/p/%s/rules/%s", rg.appID, rg.project, destinationRuleID)
		updateRequest.IssueComment = &issuetracker.IssueComment{
			Comment: fmt.Sprintf(bugs.SourceBugRuleUpdatedTemplate, bugLink),
		}
	}
	return updateRequest
}

// UpdateDuplicateDestination updates the destination bug of a
// (source, destination) duplicate bug pair, after LUCI Analysis has attempted
// to merge their failure association rules.
func (rg *RequestGenerator) UpdateDuplicateDestination(issueId int64) *issuetracker.ModifyIssueRequest {
	comment := strings.Join([]string{bugs.DestinationBugRuleUpdatedMessage, rg.linkToRuleComment(issueId)}, "\n\n")
	return &issuetracker.ModifyIssueRequest{
		IssueId: issueId,
		IssueComment: &issuetracker.IssueComment{
			Comment: comment,
		},
		AddMask: &fieldmaskpb.FieldMask{
			Paths: []string{},
		},
		Add: &issuetracker.IssueState{},
		RemoveMask: &fieldmaskpb.FieldMask{
			Paths: []string{},
		},
		Remove: &issuetracker.IssueState{},
	}
}

// NeedsUpdate determines if the bug for the given cluster needs to be updated.
func (rg *RequestGenerator) NeedsUpdate(impact *bugs.ClusterImpact,
	issue *issuetracker.Issue,
	isManagingBugPriority bool) bool {
	// Cases that a bug may be updated follow.
	switch {
	case !rg.isCompatibleWithVerified(impact, issue.IssueState.Status == issuetracker.Issue_VERIFIED):
		return true
	case isManagingBugPriority &&
		issue.IssueState.Status != issuetracker.Issue_VERIFIED &&
		!rg.isCompatibleWithPriority(impact, issue.IssueState.Priority):
		// The priority has changed on a cluster which is not verified as fixed
		// and the user isn't manually controlling the priority.
		return true
	default:
		return false
	}
}

// MakeUpdateOptions are the options for making a bug update.
type MakeUpdateOptions struct {
	// The cluster impact for the update.
	impact *bugs.ClusterImpact
	// The issue to update.
	issue *issuetracker.Issue
	// Indicates whether the rule is managing bug priority or not.
	IsManagingBugPriority bool
	// The time `IsManagingBugPriority` was last updated.
	IsManagingBugPriorityLastUpdated time.Time
}

// MakeUpdate prepares an update for the bug associated with the given impact.
// **Must** ONLY be called if NeedsUpdate(...) returns true.
func (rg *RequestGenerator) MakeUpdate(
	ctx context.Context,
	options MakeUpdateOptions) (MakeUpdateResult, error) {

	request := &issuetracker.ModifyIssueRequest{
		IssueId:      options.issue.IssueId,
		AddMask:      &fieldmaskpb.FieldMask{},
		Add:          &issuetracker.IssueState{},
		RemoveMask:   &fieldmaskpb.FieldMask{},
		Remove:       &issuetracker.IssueState{},
		IssueComment: &issuetracker.IssueComment{},
	}

	var commentary []bugs.Commentary
	result := MakeUpdateResult{}
	issueVerified := options.issue.IssueState.Status == issuetracker.Issue_VERIFIED
	if !rg.isCompatibleWithVerified(options.impact, issueVerified) {
		// Verify or reopen the issue.
		comment, err := rg.prepareBugVerifiedUpdate(ctx, options.impact, options.issue, request)
		if err != nil {
			return MakeUpdateResult{}, errors.Annotate(err, "prepare bug verified update ").Err()
		}
		commentary = append(commentary, comment)
		// After the update, whether the issue was verified will have changed.
		issueVerified = rg.clusterResolved(options.impact)
	}
	if options.IsManagingBugPriority &&
		!issueVerified &&
		!rg.isCompatibleWithPriority(options.impact, options.issue.IssueState.Priority) {
		hasManuallySetPriority, err := rg.hasManuallySetPriority(ctx, options)
		if err != nil {
			return MakeUpdateResult{}, errors.Annotate(err, "create issue update request").Err()
		}
		if hasManuallySetPriority {
			comment := bugs.Commentary{
				Body: "The bug priority has been manually set. To re-enable automatic priority updates by LUCI Analysis, enable the update priority flag on the rule.",
			}
			commentary = append(commentary, comment)
			result.disablePriorityUpdates = true
		} else {
			// We were the last to update the bug priority.
			// Apply the priority update.
			comment := rg.preparePriorityUpdate(options.impact, options.issue, request)
			commentary = append(commentary, comment)
		}
	}

	c := bugs.Commentary{
		Footer: rg.linkToRuleComment(options.issue.IssueId),
	}
	commentary = append(commentary, c)

	request.IssueComment = &issuetracker.IssueComment{
		IssueId: options.issue.IssueId,
		Comment: bugs.MergeCommentary(commentary...),
	}
	result.request = request
	return result, nil
}

// hasManuallySetPriority checks whether this issue's priority was last modified by
// a user.
func (rg *RequestGenerator) hasManuallySetPriority(ctx context.Context,
	options MakeUpdateOptions) (bool, error) {
	request := &issuetracker.ListIssueUpdatesRequest{
		IssueId: options.issue.IssueId,
	}

	it := rg.client.ListIssueUpdates(ctx, request)
	var priorityUpdateTime time.Time
	var foundUpdate bool
	// Loops on the list of the issues updates, the updates are in time-descending
	// order by default.
	for {
		update, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return false, errors.Annotate(err, "iterating through issue updates").Err()
		}
		luciAnalysisEmail := ctx.Value(&BuganizerSelfEmailKey)
		if update.Author.EmailAddress != luciAnalysisEmail {
			// If the modification was done by a user, we check if
			// the priority was updated in the list of updated fields.
			for _, fieldUpdate := range update.FieldUpdates {
				if fieldUpdate.Field == priorityField {
					foundUpdate = true
					priorityUpdateTime = update.Timestamp.AsTime()
					break
				}
			}
		}
		if foundUpdate {
			break
		}
	}
	// We compare the last time the user modified the priority was after
	// the last time the rule's priority management property was enabled.
	if foundUpdate &&
		priorityUpdateTime.After(options.IsManagingBugPriorityLastUpdated) {
		return true, nil
	}
	return false, nil
}

// prepareBugVerifiedUpdate adds bug status update to the request.
// Returns the commentary about this change.
func (rg *RequestGenerator) prepareBugVerifiedUpdate(ctx context.Context,
	impact *bugs.ClusterImpact,
	issue *issuetracker.Issue,
	request *issuetracker.ModifyIssueRequest) (bugs.Commentary, error) {
	resolved := rg.clusterResolved(impact)
	priorityMappings := rg.buganizerCfg.PriorityMappings
	var status issuetracker.Issue_Status
	var body strings.Builder
	var trailer string
	if resolved {
		// If the issue is not already closed by the user.
		if issue.IssueState.Assignee != nil {
			request.Add.Verifier = issue.IssueState.Assignee
		} else {
			if ctx.Value(&BuganizerSelfEmailKey) == nil {
				return bugs.Commentary{}, errors.New("buganizer self email is required to file buganizer bugs.")
			}
			request.Add.Verifier = &issuetracker.User{
				EmailAddress: ctx.Value(&BuganizerSelfEmailKey).(string),
			}
			request.Add.Assignee = &issuetracker.User{
				EmailAddress: ctx.Value(&BuganizerSelfEmailKey).(string),
			}
			request.AddMask.Paths = append(request.AddMask.Paths, "assignee")
		}
		status = issuetracker.Issue_VERIFIED
		request.AddMask.Paths = append(request.AddMask.Paths, "verifier")

		oldPriorityIndex := len(priorityMappings) - 1
		// A priority index of len(priorityMappings) indicates
		// a priority lower than the lowest defined priority (i.e. bug verified.)
		newPriorityIndex := len(priorityMappings)

		body.WriteString("Because:\n")
		body.WriteString(rg.priorityDecreaseJustification(oldPriorityIndex, newPriorityIndex))
		body.WriteString("LUCI Analysis is marking the issue verified.")

		trailer = fmt.Sprintf("Why issues are verified: https://%s.appspot.com/help#bug-verified", rg.appID)
	} else {
		if issue.IssueState.Assignee != nil {
			status = issuetracker.Issue_ASSIGNED
		} else {
			status = issuetracker.Issue_ACCEPTED
		}

		body.WriteString("Because:\n")
		body.WriteString(bugs.ExplainThresholdsMet(impact, rg.bugFilingThreshold))
		body.WriteString("LUCI Analysis has re-opened the bug.")

		trailer = fmt.Sprintf("Why issues are re-opened: https://%s.appspot.com/help#bug-reopened", rg.appID)
	}

	commentary := bugs.Commentary{
		Body:   body.String(),
		Footer: trailer,
	}
	request.AddMask.Paths = append(request.AddMask.Paths, "status")
	request.Add.Status = status
	return commentary, nil
}

// preparePriorityUpdate updates the issue's priority and creates a commentary for it.
func (rg *RequestGenerator) preparePriorityUpdate(impact *bugs.ClusterImpact, issue *issuetracker.Issue, request *issuetracker.ModifyIssueRequest) bugs.Commentary {
	newPriority := rg.clusterPriority(impact)
	request.AddMask.Paths = append(request.AddMask.Paths, "priority")
	request.Add.Priority = newPriority

	var body strings.Builder
	oldPriorityIndex := rg.indexOfPriority(issue.IssueState.Priority)
	newPriorityIndex := rg.indexOfPriority(newPriority)
	if newPriorityIndex < oldPriorityIndex {
		body.WriteString("Because:\n")
		body.WriteString(rg.priorityIncreaseJustification(impact, oldPriorityIndex, newPriorityIndex))
		body.WriteString(fmt.Sprintf("LUCI Analysis has increased the bug priority from %v to %v.", issue.IssueState.Priority, newPriority))
	} else {
		body.WriteString("Because:\n")
		body.WriteString(rg.priorityDecreaseJustification(oldPriorityIndex, newPriorityIndex))
		body.WriteString(fmt.Sprintf("LUCI Analysis has decreased the bug priority from %v to %v.", issue.IssueState.Priority, newPriority))
	}
	c := bugs.Commentary{
		Body:   body.String(),
		Footer: fmt.Sprintf("Why priority is updated: https://%s.appspot.com/help#priority-updated", rg.appID),
	}
	return c
}

// priorityIncreaseJustification outputs a human-readable justification
// explaining why bug priority was increased (including for the case
// where a bug was re-opened.)
//
// priorityIndex(s) are indices into the per-project priority list
// rg.buganizerCfg.PriorityMappings.
// The special index len(rg.buganizerCfg.PriorityMappings) indicates an issue
// with a priority lower than the lowest priority configured to be
// assigned by LUCI Analysis.
//
// Example output:
// "- Presubmit Runs Failed (1-day) >= 15"
func (rg *RequestGenerator) priorityIncreaseJustification(impact *bugs.ClusterImpact, oldPriorityIndex, newPriorityIndex int) string {
	if newPriorityIndex >= oldPriorityIndex {
		// Priority did not change or decreased.
		return ""
	}

	// Priority increased.
	// To justify the increase, we must show that we met the criteria for
	// each successively higher priority level.
	hysteresisPerc := rg.buganizerCfg.PriorityHysteresisPercent

	// Visit priorities in increasing priority order.
	var thresholdsMet []*configpb.ImpactThreshold
	for i := oldPriorityIndex - 1; i >= newPriorityIndex; i-- {
		metThreshold := rg.buganizerCfg.PriorityMappings[i].Threshold
		if i == oldPriorityIndex-1 {
			// For the first priority step up, we must have also exceeded
			// hysteresis.
			metThreshold = bugs.InflateThreshold(metThreshold, hysteresisPerc)
		}
		thresholdsMet = append(thresholdsMet, metThreshold)
	}
	return bugs.ExplainThresholdsMet(impact, thresholdsMet...)
}

// priorityDecreaseJustification outputs a human-readable justification
// explaining why bug priority was decreased (including to the point where
// a priority no longer applied, and the issue was marked as verified.)
//
// priorityIndex(s) are indices into the per-project priority list:
//
//	rg.projectCfg.BuganizerConfig.PriorityMappings
//
// If newPriorityIndex = len(rg.projectCfg.BuganizerConfig.PriorityMappings), it indicates
// the decrease being justified is to a priority lower than the lowest
// configured, i.e. a closed/verified issue.
//
// Example output:
// "- Presubmit Runs Failed (1-day) < 15, and
//   - Test Runs Failed (1-day) < 100"
func (rg *RequestGenerator) priorityDecreaseJustification(oldPriorityIndex, newPriorityIndex int) string {
	if newPriorityIndex <= oldPriorityIndex {
		// Priority did not change or increased.
		return ""
	}

	// Priority decreased.
	// To justify the decrease, it is sufficient to explain why we could no
	// longer meet the criteria for the next-higher priority.
	hysteresisPerc := rg.buganizerCfg.PriorityHysteresisPercent

	// The next-higher priority level that we failed to meet.
	failedToMeetThreshold := rg.buganizerCfg.PriorityMappings[newPriorityIndex-1].Threshold
	if newPriorityIndex == oldPriorityIndex+1 {
		// We only dropped one priority level. That means we failed to meet the
		// old threshold, even after applying hysteresis.
		failedToMeetThreshold = bugs.InflateThreshold(failedToMeetThreshold, -hysteresisPerc)
	}

	return bugs.ExplainThresholdNotMetMessage(failedToMeetThreshold)
}

// isCompatibleWithVerified returns whether the impact of the current cluster
// is compatible with the issue having the given verified status, based on
// configured thresholds and hysteresis.
func (rg *RequestGenerator) isCompatibleWithVerified(impact *bugs.ClusterImpact, verified bool) bool {
	hysteresisPerc := rg.buganizerCfg.PriorityHysteresisPercent
	lowestPriority := rg.buganizerCfg.PriorityMappings[len(rg.buganizerCfg.PriorityMappings)-1]
	if verified {
		// The issue is verified. Only reopen if we satisfied the bug-filing
		// criteria. Bug-filing criteria is guaranteed to imply the criteria
		// of the lowest priority level.
		return !impact.MeetsThreshold(rg.bugFilingThreshold)
	} else {
		// The issue is not verified. Only close if the impact falls
		// below the threshold with hysteresis.
		deflatedThreshold := bugs.InflateThreshold(lowestPriority.Threshold, -hysteresisPerc)
		return impact.MeetsThreshold(deflatedThreshold)
	}
}

// isCompatibleWithPriority returns whether the impact of the current cluster
// is compatible with the issue having the given priority, based on
// configured thresholds and hysteresis.
//
// If the issue's priority is not in the configuration, it is also
// considered to be incompatible.
//
// impact is the degined cluster's impact.
// priority is the issue's priority.
func (rg *RequestGenerator) isCompatibleWithPriority(impact *bugs.ClusterImpact, priority issuetracker.Issue_Priority) bool {
	priorityIndex := rg.indexOfPriority(priority)
	if priorityIndex >= len(rg.buganizerCfg.PriorityMappings) {
		// Unknown priority in use. The priority should be updated to
		// one of the configured priorities.
		return false
	}

	hysteresisPerc := rg.buganizerCfg.PriorityHysteresisPercent
	lowestAllowedPriority := rg.clusterPriorityWithInflatedThresholds(impact, hysteresisPerc)
	highestAllowedPriority := rg.clusterPriorityWithInflatedThresholds(impact, -hysteresisPerc)

	// Check that the cluster has a priority no less than lowest priority
	// and no greater than highest priority allowed by hysteresis.
	// Note that a lower priority index corresponds to a higher
	// priority (e.g. P0 <-> priorityIndex 0, P1 <-> priorityIndex 1, etc.)
	return priorityIndex <= rg.indexOfPriority(lowestAllowedPriority) &&
		priorityIndex >= rg.indexOfPriority(highestAllowedPriority)

}

// clusterResolved returns the desired state of whether the cluster has been
// verified, if no hysteresis has been applied.
func (rg *RequestGenerator) clusterResolved(impact *bugs.ClusterImpact) bool {
	lowestPriority := rg.buganizerCfg.PriorityMappings[len(rg.buganizerCfg.PriorityMappings)-1]
	return !impact.MeetsThreshold(lowestPriority.Threshold)
}

func (rg *RequestGenerator) indexOfPriority(priority issuetracker.Issue_Priority) int {
	for i, p := range rg.buganizerCfg.PriorityMappings {
		if configPriorityToIssueTrackerPriority[p.Priority] == priority {
			return i
		}
	}
	// If we can't find the priority, treat it as one lower than
	// the lowest priority we know about.
	return len(rg.buganizerCfg.PriorityMappings)
}
