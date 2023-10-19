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

// LegacyRequestGenerator generates new bugs or prepares existing ones
// for updates.
type LegacyRequestGenerator struct {
	// The issuetracker client that will be used to make RPCs to Buganizer.
	client Client
	// The LUCI project for which we are generating bug updates. This
	// is distinct from the Buganizer project.
	project string
	// The UI Base URL, e.g. "https://luci-analysis.appspot.com"
	uiBaseURL string
	// The email address the service uses to authenticate to Buganizer.
	selfEmail string
	// The Buganizer config of the LUCI project config.
	buganizerCfg *configpb.BuganizerProject
	// The threshold at which bugs are filed. Used here as the threshold
	// at which to re-open verified bugs.
	bugFilingThresholds []*configpb.ImpactMetricThreshold
}

// NewLegacyRequestGenerator initializes a new legacy buganizer request generator.
func NewLegacyRequestGenerator(
	client Client,
	project, uiBaseURL, selfEmail string,
	projectCfg *configpb.ProjectConfig) (*LegacyRequestGenerator, error) {

	return &LegacyRequestGenerator{
		client:              client,
		uiBaseURL:           uiBaseURL,
		selfEmail:           selfEmail,
		project:             project,
		buganizerCfg:        projectCfg.Buganizer,
		bugFilingThresholds: projectCfg.BugFilingThresholds,
	}, nil
}

// PrepareNewLegacy generates a CreateIssueRequest for a new issue.
// It sets the default values on the bug.
func (rg *LegacyRequestGenerator) PrepareNewLegacy(metrics bugs.ClusterMetrics,
	description *clustering.ClusterDescription,
	ruleID string,
	componentID int64) *issuetracker.CreateIssueRequest {

	issuePriority := rg.clusterPriority(metrics)

	// Justify the priority for the bug.
	thresholdComment := rg.priorityComment(metrics, issuePriority)

	ruleLink := bugs.RuleURL(rg.uiBaseURL, rg.project, ruleID)

	issue := &issuetracker.Issue{
		IssueState: &issuetracker.IssueState{
			ComponentId: componentID,
			Type:        issuetracker.Issue_BUG,
			Status:      issuetracker.Issue_NEW,
			Priority:    issuePriority,
			Severity:    issuetracker.Issue_S2,
			Title:       bugs.GenerateBugSummary(description.Title),
		},
		IssueComment: &issuetracker.IssueComment{
			Comment: bugs.NewIssueDescriptionLegacy(
				description, rg.uiBaseURL, thresholdComment, ruleLink),
		},
	}

	return &issuetracker.CreateIssueRequest{
		Issue: issue,
		TemplateOptions: &issuetracker.CreateIssueRequest_TemplateOptions{
			ApplyTemplate: true,
		},
	}
}

// clusterPriority returns the desired priority of the bug, if no hysteresis
// is applied.
func (rg *LegacyRequestGenerator) clusterPriority(metrics bugs.ClusterMetrics) issuetracker.Issue_Priority {
	return rg.clusterPriorityWithInflatedThresholds(metrics, 0)
}

// clusterPriorityWithInflatedThresholds returns the desired priority of the bug,
// if thresholds are inflated or deflated with the given percentage.
//
// See bugs.InflateThreshold for the interpretation of inflationPercent.
func (rg *LegacyRequestGenerator) clusterPriorityWithInflatedThresholds(impact bugs.ClusterMetrics, inflationPercent int64) issuetracker.Issue_Priority {
	mappings := rg.buganizerCfg.PriorityMappings
	// Default to using the lowest priorityMapping.
	priorityMapping := mappings[len(mappings)-1]
	for i := len(mappings) - 2; i >= 0; i-- {
		p := mappings[i]
		adjustedThreshold := bugs.InflateThreshold(p.Thresholds, inflationPercent)
		if !impact.MeetsAnyOfThresholds(adjustedThreshold) {
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
// issueID is the Buganizer issue ID.
func (rg *LegacyRequestGenerator) linkToRuleComment(issueID int64) string {
	ruleLink := bugs.RuleForBuganizerBugURL(rg.uiBaseURL, issueID)
	return fmt.Sprintf(bugs.LinkTemplate, ruleLink)
}

// PrepareLinkIssueCommentUpdateLegacy prepares a request that adds links to LUCI Analysis to
// a Buganizer bug by updating the issue description.
func (rg *LegacyRequestGenerator) PrepareLinkIssueCommentUpdateLegacy(metrics bugs.ClusterMetrics,
	description *clustering.ClusterDescription,
	issueID int64) *issuetracker.UpdateIssueCommentRequest {

	// Regenerate the initial comment in the same way as PrepareNew, but use
	// the link for the rule that uses the bug ID instead of the rule ID.
	issuePriority := rg.clusterPriority(metrics)
	thresholdComment := rg.priorityComment(metrics, issuePriority)
	ruleLink := bugs.RuleForBuganizerBugURL(rg.uiBaseURL, issueID)

	return &issuetracker.UpdateIssueCommentRequest{
		IssueId:       issueID,
		CommentNumber: 1,
		Comment: &issuetracker.IssueComment{
			Comment: bugs.NewIssueDescriptionLegacy(
				description, rg.uiBaseURL, thresholdComment, ruleLink),
		},
	}
}

// noPermissionComment returns a comment that explains why a bug was filed in
// the fallback component incorrectly.
//
// issueId is the Buganizer issueId.
func (rg *LegacyRequestGenerator) noPermissionComment(componentID int64) string {
	return fmt.Sprintf(bugs.NoPermissionTemplate, componentID)
}

// PrepareNoPermissionComment prepares a request that adds links to LUCI Analysis to
// a Buganizer bug.
func (rg *LegacyRequestGenerator) PrepareNoPermissionComment(issueID, componentID int64) *issuetracker.CreateIssueCommentRequest {
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
func (rg *LegacyRequestGenerator) UpdateDuplicateSource(issueID int64, errorMessage, destinationRuleID string, isAssigned bool) *issuetracker.ModifyIssueRequest {
	updateRequest := &issuetracker.ModifyIssueRequest{
		IssueId: issueID,
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
			Comment: strings.Join([]string{errorMessage, rg.linkToRuleComment(issueID)}, "\n\n"),
		}
	} else {
		ruleLink := bugs.RuleURL(rg.uiBaseURL, rg.project, destinationRuleID)
		updateRequest.IssueComment = &issuetracker.IssueComment{
			Comment: fmt.Sprintf(bugs.SourceBugRuleUpdatedTemplate, ruleLink),
		}
	}
	return updateRequest
}

// UpdateDuplicateDestination updates the destination bug of a
// (source, destination) duplicate bug pair, after LUCI Analysis has attempted
// to merge their failure association rules.
func (rg *LegacyRequestGenerator) UpdateDuplicateDestination(issueID int64) *issuetracker.ModifyIssueRequest {
	comment := strings.Join([]string{bugs.DestinationBugRuleUpdatedMessage, rg.linkToRuleComment(issueID)}, "\n\n")
	return &issuetracker.ModifyIssueRequest{
		IssueId: issueID,
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

// NeedsUpdateLegacy determines if the bug for the given cluster needs to be updated.
func (rg *LegacyRequestGenerator) NeedsUpdateLegacy(metrics bugs.ClusterMetrics,
	issue *issuetracker.Issue,
	isManagingBugPriority bool) bool {
	// Cases that a bug may be updated follow.
	switch {
	case !rg.isCompatibleWithVerifiedLegacy(metrics, issue.IssueState.Status == issuetracker.Issue_VERIFIED):
		return true
	case isManagingBugPriority &&
		issue.IssueState.Status != issuetracker.Issue_VERIFIED &&
		!rg.isCompatibleWithPriority(metrics, issue.IssueState.Priority):
		// The priority has changed on a cluster which is not verified as fixed
		// and the user isn't manually controlling the priority.
		return true
	default:
		return false
	}
}

// MakeUpdateLegacyOptions are the options for making a bug update.
type MakeUpdateLegacyOptions struct {
	// The cluster metrics.
	metrics bugs.ClusterMetrics
	// The issue to update.
	issue *issuetracker.Issue
	// Indicates whether the rule is managing bug priority or not.
	IsManagingBugPriority bool
	// The time `IsManagingBugPriority` was last updated.
	IsManagingBugPriorityLastUpdated time.Time
}

// MakeUpdateLegacy prepares an update for the bug with the given metrics.
// **Must** ONLY be called if NeedsUpdateLegacy(...) returns true.
func (rg *LegacyRequestGenerator) MakeUpdateLegacy(
	ctx context.Context,
	options MakeUpdateLegacyOptions) (MakeUpdateResult, error) {

	request := &issuetracker.ModifyIssueRequest{
		IssueId:      options.issue.IssueId,
		AddMask:      &fieldmaskpb.FieldMask{},
		Add:          &issuetracker.IssueState{},
		RemoveMask:   &fieldmaskpb.FieldMask{},
		Remove:       &issuetracker.IssueState{},
		IssueComment: &issuetracker.IssueComment{},
	}

	var commentary bugs.Commentary
	result := MakeUpdateResult{}
	issueVerified := options.issue.IssueState.Status == issuetracker.Issue_VERIFIED
	if !rg.isCompatibleWithVerifiedLegacy(options.metrics, issueVerified) {
		// Verify or reopen the issue.
		comment, err := rg.prepareBugVerifiedUpdateLegacy(ctx, options.metrics, options.issue, request)
		if err != nil {
			return MakeUpdateResult{}, errors.Annotate(err, "prepare bug verified update ").Err()
		}
		commentary = bugs.MergeCommentary(commentary, comment)
		// After the update, whether the issue was verified will have changed.
		issueVerified = rg.clusterResolved(options.metrics)

	}
	if options.IsManagingBugPriority &&
		!issueVerified &&
		!rg.isCompatibleWithPriority(options.metrics, options.issue.IssueState.Priority) {
		hasManuallySetPriority, err := rg.hasManuallySetPriorityLegacy(ctx, options)
		if err != nil {
			return MakeUpdateResult{}, errors.Annotate(err, "create issue update request").Err()
		}
		if hasManuallySetPriority {
			comment := bugs.ManualPriorityUpdateCommentary()
			commentary = bugs.MergeCommentary(commentary, comment)
			result.disablePriorityUpdates = true
		} else {
			// We were the last to update the bug priority.
			// Apply the priority update.
			comment := rg.preparePriorityUpdate(options.metrics, options.issue, request)
			commentary = bugs.MergeCommentary(commentary, comment)
		}
	}

	c := bugs.Commentary{
		Footers: []string{rg.linkToRuleComment(options.issue.IssueId)},
	}
	commentary = bugs.MergeCommentary(commentary, c)

	request.IssueComment = &issuetracker.IssueComment{
		IssueId: options.issue.IssueId,
		Comment: commentary.ToComment(),
	}
	result.request = request
	return result, nil
}

// hasManuallySetPriorityLegacy checks whether this issue's priority was last modified by
// a user.
func (rg *LegacyRequestGenerator) hasManuallySetPriorityLegacy(ctx context.Context,
	options MakeUpdateLegacyOptions) (bool, error) {
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
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return false, errors.Annotate(err, "iterating through issue updates").Err()
		}
		if update.Author.EmailAddress != rg.selfEmail {
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

// prepareBugVerifiedUpdateLegacy adds bug status update to the request.
// Returns the commentary about this change.
func (rg *LegacyRequestGenerator) prepareBugVerifiedUpdateLegacy(ctx context.Context,
	metrics bugs.ClusterMetrics,
	issue *issuetracker.Issue,
	request *issuetracker.ModifyIssueRequest) (bugs.Commentary, error) {
	resolved := rg.clusterResolved(metrics)
	priorityMappings := rg.buganizerCfg.PriorityMappings
	var status issuetracker.Issue_Status
	var body strings.Builder
	var trailer string
	if resolved {
		// If the issue is not already closed by the user.
		if issue.IssueState.Assignee != nil {
			request.Add.Verifier = issue.IssueState.Assignee
		} else {
			request.Add.Verifier = &issuetracker.User{
				EmailAddress: rg.selfEmail,
			}
			request.Add.Assignee = &issuetracker.User{
				EmailAddress: rg.selfEmail,
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

		trailer = fmt.Sprintf("Why issues are verified: %s", bugs.BugVerifiedHelpURL(rg.uiBaseURL))
	} else {
		if issue.IssueState.Assignee != nil {
			status = issuetracker.Issue_ASSIGNED
		} else {
			status = issuetracker.Issue_ACCEPTED
		}

		body.WriteString("Because:\n")
		body.WriteString(bugs.ExplainThresholdsMet(metrics, rg.bugFilingThresholds))
		body.WriteString("LUCI Analysis has re-opened the bug.")

		trailer = fmt.Sprintf("Why issues are re-opened: %s", bugs.BugReopenedHelpURL(rg.uiBaseURL))
	}

	commentary := bugs.Commentary{
		Bodies:  []string{body.String()},
		Footers: []string{trailer},
	}
	request.AddMask.Paths = append(request.AddMask.Paths, "status")
	request.Add.Status = status
	return commentary, nil
}

// preparePriorityUpdate updates the issue's priority and creates a commentary for it.
func (rg *LegacyRequestGenerator) preparePriorityUpdate(metrics bugs.ClusterMetrics, issue *issuetracker.Issue, request *issuetracker.ModifyIssueRequest) bugs.Commentary {
	newPriority := rg.clusterPriority(metrics)
	request.AddMask.Paths = append(request.AddMask.Paths, "priority")
	request.Add.Priority = newPriority

	var body strings.Builder
	oldPriorityIndex := rg.indexOfPriority(issue.IssueState.Priority)
	newPriorityIndex := rg.indexOfPriority(newPriority)
	if newPriorityIndex < oldPriorityIndex {
		body.WriteString("Because:\n")
		body.WriteString(rg.priorityIncreaseJustification(metrics, oldPriorityIndex, newPriorityIndex))
		body.WriteString(fmt.Sprintf("LUCI Analysis has increased the bug priority from %v to %v.", issue.IssueState.Priority, newPriority))
	} else {
		body.WriteString("Because:\n")
		body.WriteString(rg.priorityDecreaseJustification(oldPriorityIndex, newPriorityIndex))
		body.WriteString(fmt.Sprintf("LUCI Analysis has decreased the bug priority from %v to %v.", issue.IssueState.Priority, newPriority))
	}
	c := bugs.Commentary{
		Bodies:  []string{body.String()},
		Footers: []string{fmt.Sprintf("Why priority is updated: %s", bugs.PriorityUpdatedHelpURL(rg.uiBaseURL))},
	}
	return c
}

// priorityComment outputs a human-readable justification
// explaining why the impact justify the specified issue priority. It
// is intended to be used when bugs are initially filed.
//
// Example output:
// "The priority was set to P0 because:
// - Presubmit Runs Failed (1-day) >= 15"
func (rg *LegacyRequestGenerator) priorityComment(metrics bugs.ClusterMetrics, issuePriority issuetracker.Issue_Priority) string {
	priorityIndex := rg.indexOfPriority(issuePriority)
	if priorityIndex >= len(rg.buganizerCfg.PriorityMappings) {
		// Unknown priority - it should be one of the configured priorities.
		return ""
	}

	thresholdsMet := rg.buganizerCfg.PriorityMappings[priorityIndex].Thresholds
	justification := bugs.ExplainThresholdsMet(metrics, thresholdsMet)
	if justification == "" {
		return ""
	}

	comment := fmt.Sprintf("The priority was set to %s because:\n%s",
		issuePriority, justification)
	return strings.TrimSpace(comment)
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
func (rg *LegacyRequestGenerator) priorityIncreaseJustification(metrics bugs.ClusterMetrics, oldPriorityIndex, newPriorityIndex int) string {
	if newPriorityIndex >= oldPriorityIndex {
		// Priority did not change or decreased.
		return ""
	}

	// Priority increased.
	// To justify the increase, we must show that we met the criteria for
	// each successively higher priority level.
	hysteresisPerc := rg.buganizerCfg.PriorityHysteresisPercent

	// Visit priorities in increasing priority order.
	var thresholdsMet [][]*configpb.ImpactMetricThreshold
	for i := oldPriorityIndex - 1; i >= newPriorityIndex; i-- {
		metThreshold := rg.buganizerCfg.PriorityMappings[i].Thresholds
		if i == oldPriorityIndex-1 {
			// For the first priority step up, we must have also exceeded
			// hysteresis.
			metThreshold = bugs.InflateThreshold(metThreshold, hysteresisPerc)
		}
		thresholdsMet = append(thresholdsMet, metThreshold)
	}
	return bugs.ExplainThresholdsMet(metrics, thresholdsMet...)
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
func (rg *LegacyRequestGenerator) priorityDecreaseJustification(oldPriorityIndex, newPriorityIndex int) string {
	if newPriorityIndex <= oldPriorityIndex {
		// Priority did not change or increased.
		return ""
	}

	// Priority decreased.
	// To justify the decrease, it is sufficient to explain why we could no
	// longer meet the criteria for the next-higher priority.
	hysteresisPerc := rg.buganizerCfg.PriorityHysteresisPercent

	// The next-higher priority level that we failed to meet.
	failedToMeetThreshold := rg.buganizerCfg.PriorityMappings[newPriorityIndex-1].Thresholds
	if newPriorityIndex == oldPriorityIndex+1 {
		// We only dropped one priority level. That means we failed to meet the
		// old threshold, even after applying hysteresis.
		failedToMeetThreshold = bugs.InflateThreshold(failedToMeetThreshold, -hysteresisPerc)
	}

	return bugs.ExplainThresholdNotMetMessage(failedToMeetThreshold)
}

// isCompatibleWithVerifiedLegacy returns whether the metrics of the current cluster
// are compatible with the issue having the given verified status, based on
// configured thresholds and hysteresis.
func (rg *LegacyRequestGenerator) isCompatibleWithVerifiedLegacy(metrics bugs.ClusterMetrics, verified bool) bool {
	hysteresisPerc := rg.buganizerCfg.PriorityHysteresisPercent
	lowestPriority := rg.buganizerCfg.PriorityMappings[len(rg.buganizerCfg.PriorityMappings)-1]
	if verified {
		// The issue is verified. Only reopen if we satisfied the bug-filing
		// criteria. Bug-filing criteria is guaranteed to imply the criteria
		// of the lowest priority level.
		return !metrics.MeetsAnyOfThresholds(rg.bugFilingThresholds)
	} else {
		// The issue is not verified. Only close if the metrics fall
		// below the threshold with hysteresis.
		deflatedThreshold := bugs.InflateThreshold(lowestPriority.Thresholds, -hysteresisPerc)
		return metrics.MeetsAnyOfThresholds(deflatedThreshold)
	}
}

// isCompatibleWithPriority returns whether the metrics of the current cluster
// are compatible with the issue having the given priority, based on
// configured thresholds and hysteresis.
//
// If the issue's priority is not in the configuration, it is also
// considered to be incompatible.
func (rg *LegacyRequestGenerator) isCompatibleWithPriority(metrics bugs.ClusterMetrics, priority issuetracker.Issue_Priority) bool {
	priorityIndex := rg.indexOfPriority(priority)
	if priorityIndex >= len(rg.buganizerCfg.PriorityMappings) {
		// Unknown priority in use. The priority should be updated to
		// one of the configured priorities.
		return false
	}

	hysteresisPerc := rg.buganizerCfg.PriorityHysteresisPercent
	lowestAllowedPriority := rg.clusterPriorityWithInflatedThresholds(metrics, hysteresisPerc)
	highestAllowedPriority := rg.clusterPriorityWithInflatedThresholds(metrics, -hysteresisPerc)

	// Check that the cluster has a priority no less than lowest priority
	// and no greater than highest priority allowed by hysteresis.
	// Note that a lower priority index corresponds to a higher
	// priority (e.g. P0 <-> priorityIndex 0, P1 <-> priorityIndex 1, etc.)
	return priorityIndex <= rg.indexOfPriority(lowestAllowedPriority) &&
		priorityIndex >= rg.indexOfPriority(highestAllowedPriority)

}

// clusterResolved returns the desired state of whether the cluster has been
// verified, if no hysteresis has been applied.
func (rg *LegacyRequestGenerator) clusterResolved(metrics bugs.ClusterMetrics) bool {
	lowestPriority := rg.buganizerCfg.PriorityMappings[len(rg.buganizerCfg.PriorityMappings)-1]
	return !metrics.MeetsAnyOfThresholds(lowestPriority.Thresholds)
}

func (rg *LegacyRequestGenerator) indexOfPriority(priority issuetracker.Issue_Priority) int {
	for i, p := range rg.buganizerCfg.PriorityMappings {
		if configPriorityToIssueTrackerPriority[p.Priority] == priority {
			return i
		}
	}
	// If we can't find the priority, treat it as one lower than
	// the lowest priority we know about.
	return len(rg.buganizerCfg.PriorityMappings)
}
