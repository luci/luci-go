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

package monorail

import (
	"fmt"
	"strconv"
	"strings"

	"google.golang.org/genproto/protobuf/field_mask"

	"go.chromium.org/luci/analysis/internal/bugs"
	mpb "go.chromium.org/luci/analysis/internal/bugs/monorail/api_proto"
	"go.chromium.org/luci/analysis/internal/clustering"
	configpb "go.chromium.org/luci/analysis/proto/config"
)

// LegacyRequestGenerator provides access to methods to generate a new bug and/or bug
// updates for a cluster.
type LegacyRequestGenerator struct {
	// The UI Base URL, e.g. "https://luci-analysis.appspot.com" GAE app id, e.g. "luci-analysis".
	uiBaseURL string
	// The LUCI project for which we are generating bug updates. This
	// is distinct from the monorail project.
	project string
	// The monorail configuration to use.
	monorailCfg *configpb.MonorailProject
	// The threshold at which bugs are filed. Used here as the threshold
	// at which to re-open verified bugs.
	bugFilingThresholds []*configpb.ImpactMetricThreshold
}

// NewLegacyGenerator initialises a new LegacyRequestGenerator.
func NewLegacyGenerator(uiBaseURL, project string, projectCfg *configpb.ProjectConfig) (*LegacyRequestGenerator, error) {
	if len(projectCfg.Monorail.Priorities) == 0 {
		return nil, fmt.Errorf("invalid configuration for monorail project %q; no monorail priorities configured", projectCfg.Monorail.Project)
	}

	return &LegacyRequestGenerator{
		uiBaseURL:           uiBaseURL,
		project:             project,
		monorailCfg:         projectCfg.Monorail,
		bugFilingThresholds: projectCfg.BugFilingThresholds,
	}, nil
}

// PrepareNew prepares a new bug from the given cluster. Title and description
// are the cluster-specific bug title and description.
func (rg *LegacyRequestGenerator) PrepareNew(metrics bugs.ClusterMetrics, description *clustering.ClusterDescription, components []string) *mpb.MakeIssueRequest {
	issuePriority := rg.clusterPriority(metrics)
	issue := &mpb.Issue{
		Summary: bugs.GenerateBugSummary(description.Title),
		State:   mpb.IssueContentState_ACTIVE,
		Status:  &mpb.Issue_StatusValue{Status: UntriagedStatus},
		FieldValues: []*mpb.FieldValue{
			{
				Field: rg.priorityFieldName(),
				Value: issuePriority,
			},
		},
		Labels: []*mpb.Issue_LabelValue{{
			Label: autoFiledLabel,
		}},
	}
	if !rg.monorailCfg.FileWithoutRestrictViewGoogle {
		issue.Labels = append(issue.Labels, &mpb.Issue_LabelValue{
			Label: restrictViewLabel,
		})
	}

	for _, fv := range rg.monorailCfg.DefaultFieldValues {
		issue.FieldValues = append(issue.FieldValues, &mpb.FieldValue{
			Field: fmt.Sprintf("projects/%s/fieldDefs/%v", rg.monorailCfg.Project, fv.FieldId),
			Value: fv.Value,
		})
	}
	for _, c := range components {
		if !componentRE.MatchString(c) {
			// Discard syntactically invalid components, test results
			// cannot be trusted.
			continue
		}
		issue.Components = append(issue.Components, &mpb.Issue_ComponentValue{
			// E.g. projects/chromium/componentDefs/Blink>Workers.
			Component: fmt.Sprintf("projects/%s/componentDefs/%s", rg.monorailCfg.Project, c),
		})
	}

	// Justify the priority for the bug.
	thresholdComment := rg.priorityComment(metrics, issuePriority)

	return &mpb.MakeIssueRequest{
		Parent: fmt.Sprintf("projects/%s", rg.monorailCfg.Project),
		Issue:  issue,
		// Do not include the link to the rule in monorail initial comments,
		// as we will post it in a follow-up comment.
		Description: bugs.NewIssueDescriptionLegacy(
			description, rg.uiBaseURL, thresholdComment, "" /* ruleLink */),
		NotifyType: mpb.NotifyType_EMAIL,
	}
}

// linkToRuleComment returns a comment that links the user to the failure
// association rule in LUCI Analysis. bugName is the internal bug name,
// e.g. "chromium/100".
func (rg *LegacyRequestGenerator) linkToRuleComment(bugName string) string {
	return fmt.Sprintf(bugs.LinkTemplate, bugs.RuleForMonorailBugURL(rg.uiBaseURL, bugName))
}

// PrepareLinkComment prepares a request that adds links to LUCI Analysis to
// a monorail bug.
func (rg *LegacyRequestGenerator) PrepareLinkComment(bugID string) (*mpb.ModifyIssuesRequest, error) {
	issueName, err := toMonorailIssueName(bugID)
	if err != nil {
		return nil, err
	}

	result := &mpb.ModifyIssuesRequest{
		Deltas: []*mpb.IssueDelta{
			{
				Issue: &mpb.Issue{
					Name: issueName,
				},
				UpdateMask: &field_mask.FieldMask{},
			},
		},
		NotifyType:     mpb.NotifyType_NO_NOTIFICATION,
		CommentContent: rg.linkToRuleComment(bugID),
	}
	return result, nil
}

// UpdateDuplicateSource updates the source bug of a (source, destination)
// duplicate bug pair, after LUCI Analysis has attempted to merge their
// failure association rules.
func (rg *LegacyRequestGenerator) UpdateDuplicateSource(bugID, errorMessage, destinationRuleID string) (*mpb.ModifyIssuesRequest, error) {
	name, err := toMonorailIssueName(bugID)
	if err != nil {
		return nil, err
	}

	delta := &mpb.IssueDelta{
		Issue: &mpb.Issue{
			Name: name,
		},
		UpdateMask: &field_mask.FieldMask{
			Paths: []string{},
		},
	}
	var comment string
	if errorMessage != "" {
		delta.Issue.Status = &mpb.Issue_StatusValue{
			Status: "Available",
		}
		delta.UpdateMask.Paths = append(delta.UpdateMask.Paths, "status")
		comment = strings.Join([]string{errorMessage, rg.linkToRuleComment(bugID)}, "\n\n")
	} else {
		bugLink := bugs.RuleURL(rg.uiBaseURL, rg.project, destinationRuleID)
		comment = fmt.Sprintf(bugs.SourceBugRuleUpdatedTemplate, bugLink)
	}

	req := &mpb.ModifyIssuesRequest{
		Deltas: []*mpb.IssueDelta{
			delta,
		},
		NotifyType:     mpb.NotifyType_EMAIL,
		CommentContent: comment,
	}
	return req, nil
}

// UpdateDuplicateDestination updates the destination bug of a
// (source, destination) duplicate bug pair, after LUCI Analysis has attempted
// to merge their failure association rules.
func (rg *LegacyRequestGenerator) UpdateDuplicateDestination(bugName string) (*mpb.ModifyIssuesRequest, error) {
	name, err := toMonorailIssueName(bugName)
	if err != nil {
		return nil, err
	}

	delta := &mpb.IssueDelta{
		Issue: &mpb.Issue{
			Name: name,
		},
		UpdateMask: &field_mask.FieldMask{
			Paths: []string{},
		},
	}

	comment := strings.Join([]string{bugs.DestinationBugRuleUpdatedMessage, rg.linkToRuleComment(bugName)}, "\n\n")
	req := &mpb.ModifyIssuesRequest{
		Deltas: []*mpb.IssueDelta{
			delta,
		},
		NotifyType:     mpb.NotifyType_EMAIL,
		CommentContent: comment,
	}
	return req, nil
}

func (rg *LegacyRequestGenerator) priorityFieldName() string {
	return fmt.Sprintf("projects/%s/fieldDefs/%v", rg.monorailCfg.Project, rg.monorailCfg.PriorityFieldId)
}

// NeedsUpdate determines if the bug for the given cluster needs to be updated.
func (rg *LegacyRequestGenerator) NeedsUpdate(metrics bugs.ClusterMetrics, issue *mpb.Issue, isManagingBugPriority bool) bool {
	// Cases that a bug may be updated follow.
	switch {
	case !rg.isCompatibleWithVerified(metrics, issueVerified(issue)):
		return true
	case isManagingBugPriority &&
		!issueVerified(issue) &&
		!rg.isCompatibleWithPriority(metrics, rg.IssuePriority(issue)):
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
	issue *mpb.Issue
	// Indicates whether the rule is managing bug priority or not.
	// Use the value on the rule; do not yet set it to false if
	// HasManuallySetPriority is true.
	IsManagingBugPriority bool
	// Whether the user has manually taken control of the bug priority.
	HasManuallySetPriority bool
}

// MakeUpdate prepares an updated for the bug associated with a given cluster.
// Must ONLY be called if NeedsUpdate(...) returns true.
func (rg *LegacyRequestGenerator) MakeUpdate(options MakeUpdateLegacyOptions) MakeUpdateResult {
	delta := &mpb.IssueDelta{
		Issue: &mpb.Issue{
			Name: options.issue.Name,
		},
		UpdateMask: &field_mask.FieldMask{
			Paths: []string{},
		},
	}

	var commentary bugs.Commentary
	notify := false
	issueVerified := issueVerified(options.issue)
	if !rg.isCompatibleWithVerified(options.metrics, issueVerified) {
		// Verify or reopen the issue.
		commentary = rg.prepareBugVerifiedUpdate(options.metrics, options.issue, delta)
		notify = true
		// After the update, whether the issue was verified will have changed.
		issueVerified = rg.clusterResolved(options.metrics)
	}
	disablePriorityUpdates := false
	if options.IsManagingBugPriority &&
		!issueVerified &&
		!rg.isCompatibleWithPriority(options.metrics, rg.IssuePriority(options.issue)) {
		if options.HasManuallySetPriority {
			// We were not the last to update the priority of this issue.
			// We need to turn the flag isManagingBugPriority off on the rule.
			comment := bugs.ManualPriorityUpdateCommentary()
			commentary = bugs.MergeCommentary(commentary, comment)
			disablePriorityUpdates = true
		} else {
			// We were the last to update the bug priority.
			// Apply the priority update.
			comment := rg.preparePriorityUpdate(options.metrics, options.issue, delta)
			commentary = bugs.MergeCommentary(commentary, comment)
			// Notify if new priority is higher than existing priority.
			notify = notify || rg.isHigherPriority(rg.clusterPriority(options.metrics), rg.IssuePriority(options.issue))
		}
	}

	bugName, err := fromMonorailIssueName(options.issue.Name)
	if err != nil {
		// This should never happen. It would mean monorail is feeding us
		// invalid data.
		panic("invalid monorail issue name: " + options.issue.Name)
	}

	commentary.Footers = append(commentary.Footers, rg.linkToRuleComment(bugName))

	update := &mpb.ModifyIssuesRequest{
		Deltas: []*mpb.IssueDelta{
			delta,
		},
		NotifyType:     mpb.NotifyType_NO_NOTIFICATION,
		CommentContent: commentary.ToComment(),
	}
	if notify {
		update.NotifyType = mpb.NotifyType_EMAIL
	}
	return MakeUpdateResult{
		request:                   update,
		disableBugPriorityUpdates: disablePriorityUpdates,
	}
}

func (rg *LegacyRequestGenerator) prepareBugVerifiedUpdate(metrics bugs.ClusterMetrics, issue *mpb.Issue, update *mpb.IssueDelta) bugs.Commentary {
	resolved := rg.clusterResolved(metrics)
	var status string
	var body strings.Builder
	var trailer string
	if resolved {
		status = VerifiedStatus

		oldPriorityIndex := len(rg.monorailCfg.Priorities) - 1
		// A priority index of len(g.monorailCfg.Priorities) indicates
		// a priority lower than the lowest defined priority (i.e. bug verified.)
		newPriorityIndex := len(rg.monorailCfg.Priorities)

		body.WriteString("Because:\n")
		body.WriteString(rg.priorityDecreaseJustification(oldPriorityIndex, newPriorityIndex))
		body.WriteString("LUCI Analysis is marking the issue verified.")

		trailer = fmt.Sprintf("Why issues are verified and how to stop automatic verification: %s", bugs.BugVerifiedHelpURL(rg.uiBaseURL))
	} else {
		if issue.GetOwner().GetUser() != "" {
			status = AssignedStatus
		} else {
			status = UntriagedStatus
		}

		body.WriteString("Because:\n")
		body.WriteString(bugs.ExplainThresholdsMet(metrics, rg.bugFilingThresholds))
		body.WriteString("LUCI Analysis has re-opened the bug.")

		trailer = fmt.Sprintf("Why issues are re-opened: %s", bugs.BugReopenedHelpURL(rg.uiBaseURL))
	}
	update.Issue.Status = &mpb.Issue_StatusValue{Status: status}
	update.UpdateMask.Paths = append(update.UpdateMask.Paths, "status")

	c := bugs.Commentary{
		Bodies:  []string{body.String()},
		Footers: []string{trailer},
	}
	return c
}

func (rg *LegacyRequestGenerator) preparePriorityUpdate(metrics bugs.ClusterMetrics, issue *mpb.Issue, update *mpb.IssueDelta) bugs.Commentary {
	newPriority := rg.clusterPriority(metrics)

	update.Issue.FieldValues = []*mpb.FieldValue{
		{
			Field: rg.priorityFieldName(),
			Value: newPriority,
		},
	}
	update.UpdateMask.Paths = append(update.UpdateMask.Paths, "field_values")

	oldPriority := rg.IssuePriority(issue)
	oldPriorityIndex := rg.indexOfPriority(oldPriority)
	newPriorityIndex := rg.indexOfPriority(newPriority)

	var body strings.Builder
	if newPriorityIndex < oldPriorityIndex {
		body.WriteString("Because:\n")
		body.WriteString(rg.priorityIncreaseJustification(metrics, oldPriorityIndex, newPriorityIndex))
		body.WriteString(fmt.Sprintf("LUCI Analysis has increased the bug priority from %v to %v.", oldPriority, newPriority))
	} else {
		body.WriteString("Because:\n")
		body.WriteString(rg.priorityDecreaseJustification(oldPriorityIndex, newPriorityIndex))
		body.WriteString(fmt.Sprintf("LUCI Analysis has decreased the bug priority from %v to %v.", oldPriority, newPriority))
	}
	c := bugs.Commentary{
		Bodies:  []string{body.String()},
		Footers: []string{fmt.Sprintf("Why priority is updated: %s", bugs.PriorityUpdatedHelpURL(rg.uiBaseURL))},
	}
	return c
}

// IssuePriority returns the priority of the given issue.
func (rg *LegacyRequestGenerator) IssuePriority(issue *mpb.Issue) string {
	priorityFieldName := rg.priorityFieldName()
	for _, fv := range issue.FieldValues {
		if fv.Field == priorityFieldName {
			return fv.Value
		}
	}
	return ""
}

// isHigherPriority returns whether priority p1 is higher than priority p2.
// The passed strings are the priority field values as used in monorail. These
// must be matched against monorail project configuration in order to
// identify the ordering of the priorities.
func (rg *LegacyRequestGenerator) isHigherPriority(p1 string, p2 string) bool {
	i1 := rg.indexOfPriority(p1)
	i2 := rg.indexOfPriority(p2)
	// Priorities are configured from highest to lowest, so higher priorities
	// have lower indexes.
	return i1 < i2
}

func (rg *LegacyRequestGenerator) indexOfPriority(priority string) int {
	for i, p := range rg.monorailCfg.Priorities {
		if p.Priority == priority {
			return i
		}
	}
	// If we can't find the priority, treat it as one lower than
	// the lowest priority we know about.
	return len(rg.monorailCfg.Priorities)
}

// isCompatibleWithVerified returns whether the metrics of the current cluster
// are compatible with the issue having the given verified status, based on
// configured thresholds and hysteresis.
func (rg *LegacyRequestGenerator) isCompatibleWithVerified(metrics bugs.ClusterMetrics, verified bool) bool {
	hysteresisPerc := rg.monorailCfg.PriorityHysteresisPercent
	lowestPriority := rg.monorailCfg.Priorities[len(rg.monorailCfg.Priorities)-1]
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
// An unknown priority that is not in the configuration will be considered
// Incompatible.
func (rg *LegacyRequestGenerator) isCompatibleWithPriority(metrics bugs.ClusterMetrics, issuePriority string) bool {
	index := rg.indexOfPriority(issuePriority)
	if index >= len(rg.monorailCfg.Priorities) {
		// Unknown priority in use. The priority should be updated to
		// one of the configured priorities.
		return false
	}
	hysteresisPerc := rg.monorailCfg.PriorityHysteresisPercent
	lowestAllowedPriority := rg.clusterPriorityWithInflatedThresholds(metrics, hysteresisPerc)
	highestAllowedPriority := rg.clusterPriorityWithInflatedThresholds(metrics, -hysteresisPerc)

	// Check the cluster has a priority no less than lowest priority
	// and no greater than highest priority allowed by hysteresis.
	// Note that a lower priority index corresponds to a higher
	// priority (e.g. P0 <-> index 0, P1 <-> index 1, etc.)
	return rg.indexOfPriority(lowestAllowedPriority) >= index &&
		index >= rg.indexOfPriority(highestAllowedPriority)
}

// priorityComment outputs a human-readable justification
// explaining why the metrics justify the specified issue priority. It
// is intended to be used when bugs are initially filed.
//
// Example output:
// "The priority was set to P0 because:
// - Test Results Failed (1-day) >= 500"
func (rg *LegacyRequestGenerator) priorityComment(metrics bugs.ClusterMetrics, issuePriority string) string {
	priorityIndex := rg.indexOfPriority(issuePriority)
	if priorityIndex >= len(rg.monorailCfg.Priorities) {
		// Unknown priority - it should be one of the configured priorities.
		return ""
	}

	thresholdsMet := rg.monorailCfg.Priorities[priorityIndex].Thresholds
	justification := bugs.ExplainThresholdsMet(metrics, thresholdsMet)
	if justification == "" {
		return ""
	}

	priorityPrefix := ""
	if _, err := strconv.Atoi(issuePriority); err == nil {
		// The priority is a bare integer (e.g. "1"), so let's use a prefix to
		// denote it is a priority.
		priorityPrefix = "P"
	}

	comment := fmt.Sprintf(
		"The priority was set to %s%s because:\n%s",
		priorityPrefix, strings.TrimSpace(issuePriority), justification)
	return strings.TrimSpace(comment)
}

// priorityDecreaseJustification outputs a human-readable justification
// explaining why bug priority was decreased (including to the point where
// a priority no longer applied, and the issue was marked as verified.)
//
// priorityIndex(s) are indices into the per-project priority list:
//
//	g.monorailCfg.Priorities
//
// If newPriorityIndex = len(g.monorailCfg.Priorities), it indicates
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
	hysteresisPerc := rg.monorailCfg.PriorityHysteresisPercent

	// The next-higher priority level that we failed to meet.
	failedToMeetThreshold := rg.monorailCfg.Priorities[newPriorityIndex-1].Thresholds
	if newPriorityIndex == oldPriorityIndex+1 {
		// We only dropped one priority level. That means we failed to meet the
		// old threshold, even after applying hysteresis.
		failedToMeetThreshold = bugs.InflateThreshold(failedToMeetThreshold, -hysteresisPerc)
	}

	return bugs.ExplainThresholdNotMetMessage(failedToMeetThreshold)
}

// priorityIncreaseJustification outputs a human-readable justification
// explaining why bug priority was increased (including for the case
// where a bug was re-opened.)
//
// priorityIndex(s) are indices into the per-project priority list:
//
//	g.monorailCfg.Priorities
//
// The special index len(g.monorailCfg.Priorities) indicates an issue
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
	hysteresisPerc := rg.monorailCfg.PriorityHysteresisPercent

	// Visit priorities in increasing priority order.
	var thresholdsMet [][]*configpb.ImpactMetricThreshold
	for i := oldPriorityIndex - 1; i >= newPriorityIndex; i-- {
		metThreshold := rg.monorailCfg.Priorities[i].Thresholds
		if i == oldPriorityIndex-1 {
			// For the first priority step up, we must have also exceeded
			// hysteresis.
			metThreshold = bugs.InflateThreshold(metThreshold, hysteresisPerc)
		}
		thresholdsMet = append(thresholdsMet, metThreshold)
	}
	return bugs.ExplainThresholdsMet(metrics, thresholdsMet...)
}

// clusterPriority returns the desired priority of the bug, if no hysteresis
// is applied.
func (rg *LegacyRequestGenerator) clusterPriority(metrics bugs.ClusterMetrics) string {
	return rg.clusterPriorityWithInflatedThresholds(metrics, 0)
}

// clusterPriorityWithInflatedThresholds returns the desired priority of the bug,
// if thresholds are inflated or deflated with the given percentage.
//
// See bugs.InflateThreshold for the interpretation of inflationPercent.
func (rg *LegacyRequestGenerator) clusterPriorityWithInflatedThresholds(metrics bugs.ClusterMetrics, inflationPercent int64) string {
	// Default to using the lowest priority.
	priority := rg.monorailCfg.Priorities[len(rg.monorailCfg.Priorities)-1]
	for i := len(rg.monorailCfg.Priorities) - 2; i >= 0; i-- {
		p := rg.monorailCfg.Priorities[i]
		adjustedThreshold := bugs.InflateThreshold(p.Thresholds, inflationPercent)
		if !metrics.MeetsAnyOfThresholds(adjustedThreshold) {
			// A cluster cannot reach a higher priority unless it has
			// met the thresholds for all lower priorities.
			break
		}
		priority = p
	}
	return priority.Priority
}

// clusterResolved returns the desired state of whether the cluster has been
// verified, if no hysteresis has been applied.
func (rg *LegacyRequestGenerator) clusterResolved(metrics bugs.ClusterMetrics) bool {
	lowestPriority := rg.monorailCfg.Priorities[len(rg.monorailCfg.Priorities)-1]
	return !metrics.MeetsAnyOfThresholds(lowestPriority.Thresholds)
}
