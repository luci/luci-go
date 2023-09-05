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
	"regexp"
	"strconv"
	"strings"
	"time"

	"google.golang.org/genproto/protobuf/field_mask"

	"go.chromium.org/luci/analysis/internal/bugs"
	mpb "go.chromium.org/luci/analysis/internal/bugs/monorail/api_proto"
	"go.chromium.org/luci/analysis/internal/clustering"
	configpb "go.chromium.org/luci/analysis/proto/config"
)

const (
	restrictViewLabel = "Restrict-View-Google"
	autoFiledLabel    = "LUCI-Analysis-Auto-Filed"
)

// priorityRE matches chromium monorail priority values.
var priorityRE = regexp.MustCompile(`^Pri-([0123])$`)

// AutomationUsers are the identifiers of LUCI Analysis automation users
// in monorail.
var AutomationUsers = []string{
	"users/3816576959", // chops-weetbix@appspot.gserviceaccount.com
	"users/4149141945", // chops-weetbix-dev@appspot.gserviceaccount.com
	"users/3371420746", // luci-analysis@appspot.gserviceaccount.com
	"users/641595106",  // luci-analysis-service@luci-analysis.iam.gserviceaccount.com
	"users/1503471452", // luci-analysis-dev@appspot.gserviceaccount.com
	"users/3692272382", // luci-analysis-dev-service@luci-analysis-dev.iam.gserviceaccount.com
}

// VerifiedStatus is that status of bugs that have been fixed and verified.
const VerifiedStatus = "Verified"

// AssignedStatus is the status of bugs that are open and assigned to an owner.
const AssignedStatus = "Assigned"

// UntriagedStatus is the status of bugs that have just been opened.
const UntriagedStatus = "Untriaged"

// DuplicateStatus is the status of bugs which are closed as duplicate.
const DuplicateStatus = "Duplicate"

// FixedStatus is the status of bugs which have been fixed, but not verified.
const FixedStatus = "Fixed"

// ClosedStatuses is the status of bugs which are closed.
// Comprises statuses configured on chromium and fuchsia projects. Ideally
// this would be configuration, but given the coming monorail deprecation,
// there is limited value.
var ClosedStatuses = map[string]struct{}{
	"Fixed":           {},
	"Verified":        {},
	"WontFix":         {},
	"Done":            {},
	"NotReproducible": {},
	"Archived":        {},
	"Obsolete":        {},
}

// ArchivedStatuses is the subset of closed statuses that indicate a bug
// that should no longer be used.
var ArchivedStatuses = map[string]struct{}{
	"Archived": {},
	"Obsolete": {},
}

// Generator provides access to methods to generate a new bug and/or bug
// updates for a cluster.
type Generator struct {
	// The GAE app id, e.g. "luci-analysis".
	appID string
	// The LUCI project for which we are generating bug updates. This
	// is distinct from the monorail project.
	project string
	// The monorail configuration to use.
	monorailCfg *configpb.MonorailProject
	// The threshold at which bugs are filed. Used here as the threshold
	// at which to re-open verified bugs.
	bugFilingThresholds []*configpb.ImpactMetricThreshold
}

// NewGenerator initialises a new Generator.
func NewGenerator(appID, project string, projectCfg *configpb.ProjectConfig) (*Generator, error) {
	if len(projectCfg.Monorail.Priorities) == 0 {
		return nil, fmt.Errorf("invalid configuration for monorail project %q; no monorail priorities configured", projectCfg.Monorail.Project)
	}
	return &Generator{
		appID:               appID,
		project:             project,
		monorailCfg:         projectCfg.Monorail,
		bugFilingThresholds: projectCfg.BugFilingThresholds,
	}, nil
}

// PrepareNew prepares a new bug from the given cluster. Title and description
// are the cluster-specific bug title and description.
func (g *Generator) PrepareNew(metrics *bugs.ClusterMetrics, description *clustering.ClusterDescription, components []string) *mpb.MakeIssueRequest {
	issuePriority := g.clusterPriority(metrics)
	issue := &mpb.Issue{
		Summary: bugs.GenerateBugSummary(description.Title),
		State:   mpb.IssueContentState_ACTIVE,
		Status:  &mpb.Issue_StatusValue{Status: UntriagedStatus},
		FieldValues: []*mpb.FieldValue{
			{
				Field: g.priorityFieldName(),
				Value: issuePriority,
			},
		},
		Labels: []*mpb.Issue_LabelValue{{
			Label: autoFiledLabel,
		}},
	}
	if !g.monorailCfg.FileWithoutRestrictViewGoogle {
		issue.Labels = append(issue.Labels, &mpb.Issue_LabelValue{
			Label: restrictViewLabel,
		})
	}

	for _, fv := range g.monorailCfg.DefaultFieldValues {
		issue.FieldValues = append(issue.FieldValues, &mpb.FieldValue{
			Field: fmt.Sprintf("projects/%s/fieldDefs/%v", g.monorailCfg.Project, fv.FieldId),
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
			Component: fmt.Sprintf("projects/%s/componentDefs/%s", g.monorailCfg.Project, c),
		})
	}

	// Justify the priority for the bug.
	thresholdComment := g.priorityComment(metrics, issuePriority)

	return &mpb.MakeIssueRequest{
		Parent: fmt.Sprintf("projects/%s", g.monorailCfg.Project),
		Issue:  issue,
		// Do not include the link to the rule in monorail initial comments,
		// as we will post it in a follow-up comment.
		Description: bugs.GenerateInitialIssueDescription(
			description, g.appID, thresholdComment, "" /* ruleLink */),
		NotifyType: mpb.NotifyType_EMAIL,
	}
}

// linkToRuleComment returns a comment that links the user to the failure
// association rule in LUCI Analysis. bugName is the internal bug name,
// e.g. "chromium/100".
func (g *Generator) linkToRuleComment(bugName string) string {
	bugLink := fmt.Sprintf("https://%s.appspot.com/b/%s", g.appID, bugName)
	return fmt.Sprintf(bugs.LinkTemplate, bugLink)
}

// PrepareLinkComment prepares a request that adds links to LUCI Analysis to
// a monorail bug.
func (g *Generator) PrepareLinkComment(bugName string) (*mpb.ModifyIssuesRequest, error) {
	issueName, err := toMonorailIssueName(bugName)
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
		CommentContent: g.linkToRuleComment(bugName),
	}
	return result, nil
}

// UpdateDuplicateSource updates the source bug of a (source, destination)
// duplicate bug pair, after LUCI Analysis has attempted to merge their
// failure association rules.
func (g *Generator) UpdateDuplicateSource(bugName, errorMessage, destinationRuleID string) (*mpb.ModifyIssuesRequest, error) {
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
	var comment string
	if errorMessage != "" {
		delta.Issue.Status = &mpb.Issue_StatusValue{
			Status: "Available",
		}
		delta.UpdateMask.Paths = append(delta.UpdateMask.Paths, "status")
		comment = strings.Join([]string{errorMessage, g.linkToRuleComment(bugName)}, "\n\n")
	} else {
		bugLink := fmt.Sprintf("https://%s.appspot.com/p/%s/rules/%s", g.appID, g.project, destinationRuleID)
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
func (g *Generator) UpdateDuplicateDestination(bugName string) (*mpb.ModifyIssuesRequest, error) {
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

	comment := strings.Join([]string{bugs.DestinationBugRuleUpdatedMessage, g.linkToRuleComment(bugName)}, "\n\n")
	req := &mpb.ModifyIssuesRequest{
		Deltas: []*mpb.IssueDelta{
			delta,
		},
		NotifyType:     mpb.NotifyType_EMAIL,
		CommentContent: comment,
	}
	return req, nil
}

func (g *Generator) priorityFieldName() string {
	return fmt.Sprintf("projects/%s/fieldDefs/%v", g.monorailCfg.Project, g.monorailCfg.PriorityFieldId)
}

// NeedsUpdate determines if the bug for the given cluster needs to be updated.
func (g *Generator) NeedsUpdate(metrics *bugs.ClusterMetrics, issue *mpb.Issue, isManagingBugPriority bool) bool {
	// Cases that a bug may be updated follow.
	switch {
	case !g.isCompatibleWithVerified(metrics, issueVerified(issue)):
		return true
	case isManagingBugPriority &&
		!issueVerified(issue) &&
		!g.isCompatibleWithPriority(metrics, g.IssuePriority(issue)):
		// The priority has changed on a cluster which is not verified as fixed
		// and the user isn't manually controlling the priority.
		return true
	default:
		return false
	}
}

// MakeUpdateOptions are the options for making a bug update.
type MakeUpdateOptions struct {
	// The cluster metrics.
	metrics *bugs.ClusterMetrics
	// The issue to update.
	issue *mpb.Issue
	// Monorail issue comments for the provided issue.
	comments []*mpb.Comment
	// Indicates whether the rule is managing bug priority or not.
	IsManagingBugPriority bool
	// The time `IsManagingBugPriority` was last updated.
	IsManagingBugPriorityLastUpdated time.Time
}

// The result of a MakeUpdate request.
type MakeUpdateResult struct {
	// The created request to modify an issue
	request *mpb.ModifyIssuesRequest

	// Whether isManagingBugPriority should be disabled on the rule.
	// This is determined by checking if the last time the flag was turned on
	// was after the last time a user updated the priority manually.
	disableBugPriorityUpdates bool
}

// MakeUpdate prepares an updated for the bug associated with a given cluster.
// Must ONLY be called if NeedsUpdate(...) returns true.
func (g *Generator) MakeUpdate(options MakeUpdateOptions) MakeUpdateResult {
	delta := &mpb.IssueDelta{
		Issue: &mpb.Issue{
			Name: options.issue.Name,
		},
		UpdateMask: &field_mask.FieldMask{
			Paths: []string{},
		},
	}

	var commentary []bugs.Commentary
	notify := false
	issueVerified := issueVerified(options.issue)
	if !g.isCompatibleWithVerified(options.metrics, issueVerified) {
		// Verify or reopen the issue.
		comment := g.prepareBugVerifiedUpdate(options.metrics, options.issue, delta)
		commentary = append(commentary, comment)
		notify = true
		// After the update, whether the issue was verified will have changed.
		issueVerified = g.clusterResolved(options.metrics)
	}
	disablePriorityUpdates := false
	if options.IsManagingBugPriority &&
		!issueVerified &&
		!g.isCompatibleWithPriority(options.metrics, g.IssuePriority(options.issue)) {
		if hasManuallySetPriority(options.comments, options.IsManagingBugPriorityLastUpdated) {
			// We were not the last to update the priority of this issue.
			// We need to turn the flag isManagingBugPriority off on the rule.
			comment := prepareManualPriorityUpdate(options.issue, delta)
			commentary = append(commentary, comment)
			disablePriorityUpdates = true
		} else {
			// We were the last to update the bug priority.
			// Apply the priority update.
			comment := g.preparePriorityUpdate(options.metrics, options.issue, delta)
			commentary = append(commentary, comment)
			// Notify if new priority is higher than existing priority.
			notify = notify || g.isHigherPriority(g.clusterPriority(options.metrics), g.IssuePriority(options.issue))
		}
	}

	bugName, err := fromMonorailIssueName(options.issue.Name)
	if err != nil {
		// This should never happen. It would mean monorail is feeding us
		// invalid data.
		panic("invalid monorail issue name: " + options.issue.Name)
	}

	c := bugs.Commentary{
		Footer: g.linkToRuleComment(bugName),
	}
	commentary = append(commentary, c)

	update := &mpb.ModifyIssuesRequest{
		Deltas: []*mpb.IssueDelta{
			delta,
		},
		NotifyType:     mpb.NotifyType_NO_NOTIFICATION,
		CommentContent: bugs.MergeCommentary(commentary...),
	}
	if notify {
		update.NotifyType = mpb.NotifyType_EMAIL
	}
	return MakeUpdateResult{
		request:                   update,
		disableBugPriorityUpdates: disablePriorityUpdates,
	}
}

func (g *Generator) prepareBugVerifiedUpdate(metrics *bugs.ClusterMetrics, issue *mpb.Issue, update *mpb.IssueDelta) bugs.Commentary {
	resolved := g.clusterResolved(metrics)
	var status string
	var body strings.Builder
	var trailer string
	if resolved {
		status = VerifiedStatus

		oldPriorityIndex := len(g.monorailCfg.Priorities) - 1
		// A priority index of len(g.monorailCfg.Priorities) indicates
		// a priority lower than the lowest defined priority (i.e. bug verified.)
		newPriorityIndex := len(g.monorailCfg.Priorities)

		body.WriteString("Because:\n")
		body.WriteString(g.priorityDecreaseJustification(oldPriorityIndex, newPriorityIndex))
		body.WriteString("LUCI Analysis is marking the issue verified.")

		trailer = fmt.Sprintf("Why issues are verified and how to stop automatic verification: https://%s.appspot.com/help#bug-verified", g.appID)
	} else {
		if issue.GetOwner().GetUser() != "" {
			status = AssignedStatus
		} else {
			status = UntriagedStatus
		}

		body.WriteString("Because:\n")
		body.WriteString(bugs.ExplainThresholdsMet(metrics, g.bugFilingThresholds))
		body.WriteString("LUCI Analysis has re-opened the bug.")

		trailer = fmt.Sprintf("Why issues are re-opened: https://%s.appspot.com/help#bug-reopened", g.appID)
	}
	update.Issue.Status = &mpb.Issue_StatusValue{Status: status}
	update.UpdateMask.Paths = append(update.UpdateMask.Paths, "status")

	c := bugs.Commentary{
		Body:   body.String(),
		Footer: trailer,
	}
	return c
}

func prepareManualPriorityUpdate(issue *mpb.Issue, update *mpb.IssueDelta) bugs.Commentary {
	c := bugs.Commentary{
		Body: "The bug priority has been manually set. To re-enable automatic priority updates by LUCI Analysis, enable the update priority flag on the rule.",
	}
	return c
}

func (g *Generator) preparePriorityUpdate(metrics *bugs.ClusterMetrics, issue *mpb.Issue, update *mpb.IssueDelta) bugs.Commentary {
	newPriority := g.clusterPriority(metrics)

	update.Issue.FieldValues = []*mpb.FieldValue{
		{
			Field: g.priorityFieldName(),
			Value: newPriority,
		},
	}
	update.UpdateMask.Paths = append(update.UpdateMask.Paths, "field_values")

	oldPriority := g.IssuePriority(issue)
	oldPriorityIndex := g.indexOfPriority(oldPriority)
	newPriorityIndex := g.indexOfPriority(newPriority)

	var body strings.Builder
	if newPriorityIndex < oldPriorityIndex {
		body.WriteString("Because:\n")
		body.WriteString(g.priorityIncreaseJustification(metrics, oldPriorityIndex, newPriorityIndex))
		body.WriteString(fmt.Sprintf("LUCI Analysis has increased the bug priority from %v to %v.", oldPriority, newPriority))
	} else {
		body.WriteString("Because:\n")
		body.WriteString(g.priorityDecreaseJustification(oldPriorityIndex, newPriorityIndex))
		body.WriteString(fmt.Sprintf("LUCI Analysis has decreased the bug priority from %v to %v.", oldPriority, newPriority))
	}
	c := bugs.Commentary{
		Body:   body.String(),
		Footer: fmt.Sprintf("Why priority is updated: https://%s.appspot.com/help#priority-updated", g.appID),
	}
	return c
}

// hasManuallySetPriority returns whether the given issue has a manually
// controlled priority, based on its comments and the last time the isManagingBugPrirotiy
// was last updated on the rule.
func hasManuallySetPriority(comments []*mpb.Comment, isManagingBugPriorityLastUpdated time.Time) bool {
	// Example comment showing a user changing priority:
	// {
	// 	name: "projects/chromium/issues/915761/comments/1"
	// 	state: ACTIVE
	// 	type: COMMENT
	// 	commenter: "users/2627516260"
	// 	create_time: {
	// 	  seconds: 1632111572
	// 	}
	// 	amendments: {
	// 	  field_name: "Labels"
	// 	  new_or_delta_value: "Pri-1"
	// 	}
	// }

	foundManualPriorityUpdate := false
	var manualPriorityUpdateTime time.Time
outer:
	for i := len(comments) - 1; i >= 0; i-- {
		c := comments[i]
		for _, a := range c.Amendments {
			if a.FieldName == "Labels" {
				deltaLabels := strings.Split(a.NewOrDeltaValue, " ")
				for _, lbl := range deltaLabels {
					if priorityRE.MatchString(lbl) && !isAutomationUser(c.Commenter) {
						manualPriorityUpdateTime = c.CreateTime.AsTime()
						foundManualPriorityUpdate = true
						break outer
					}
				}
			}
		}
	}
	// If there is a manual priority update
	// after the managing bug priority was last updated.
	if foundManualPriorityUpdate &&
		manualPriorityUpdateTime.After(isManagingBugPriorityLastUpdated) {
		return true
	}
	// No manual changes to priority indicates the bug is still under
	// automatic control.
	return false
}

func isAutomationUser(user string) bool {
	for _, u := range AutomationUsers {
		if u == user {
			return true
		}
	}
	return false
}

// IssuePriority returns the priority of the given issue.
func (g *Generator) IssuePriority(issue *mpb.Issue) string {
	priorityFieldName := g.priorityFieldName()
	for _, fv := range issue.FieldValues {
		if fv.Field == priorityFieldName {
			return fv.Value
		}
	}
	return ""
}

func issueVerified(issue *mpb.Issue) bool {
	return issue.Status.Status == VerifiedStatus
}

// isHigherPriority returns whether priority p1 is higher than priority p2.
// The passed strings are the priority field values as used in monorail. These
// must be matched against monorail project configuration in order to
// identify the ordering of the priorities.
func (g *Generator) isHigherPriority(p1 string, p2 string) bool {
	i1 := g.indexOfPriority(p1)
	i2 := g.indexOfPriority(p2)
	// Priorities are configured from highest to lowest, so higher priorities
	// have lower indexes.
	return i1 < i2
}

func (g *Generator) indexOfPriority(priority string) int {
	for i, p := range g.monorailCfg.Priorities {
		if p.Priority == priority {
			return i
		}
	}
	// If we can't find the priority, treat it as one lower than
	// the lowest priority we know about.
	return len(g.monorailCfg.Priorities)
}

// isCompatibleWithVerified returns whether the metrics of the current cluster
// are compatible with the issue having the given verified status, based on
// configured thresholds and hysteresis.
func (g *Generator) isCompatibleWithVerified(metrics *bugs.ClusterMetrics, verified bool) bool {
	hysteresisPerc := g.monorailCfg.PriorityHysteresisPercent
	lowestPriority := g.monorailCfg.Priorities[len(g.monorailCfg.Priorities)-1]
	if verified {
		// The issue is verified. Only reopen if we satisfied the bug-filing
		// criteria. Bug-filing criteria is guaranteed to imply the criteria
		// of the lowest priority level.
		return !metrics.MeetsAnyOfThresholds(g.bugFilingThresholds)
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
func (g *Generator) isCompatibleWithPriority(metrics *bugs.ClusterMetrics, issuePriority string) bool {
	index := g.indexOfPriority(issuePriority)
	if index >= len(g.monorailCfg.Priorities) {
		// Unknown priority in use. The priority should be updated to
		// one of the configured priorities.
		return false
	}
	hysteresisPerc := g.monorailCfg.PriorityHysteresisPercent
	lowestAllowedPriority := g.clusterPriorityWithInflatedThresholds(metrics, hysteresisPerc)
	highestAllowedPriority := g.clusterPriorityWithInflatedThresholds(metrics, -hysteresisPerc)

	// Check the cluster has a priority no less than lowest priority
	// and no greater than highest priority allowed by hysteresis.
	// Note that a lower priority index corresponds to a higher
	// priority (e.g. P0 <-> index 0, P1 <-> index 1, etc.)
	return g.indexOfPriority(lowestAllowedPriority) >= index &&
		index >= g.indexOfPriority(highestAllowedPriority)
}

// priorityComment outputs a human-readable justification
// explaining why the metrics justify the specified issue priority. It
// is intended to be used when bugs are initially filed.
//
// Example output:
// "The priority was set to P0 because:
// - Test Results Failed (1-day) >= 500"
func (g *Generator) priorityComment(metrics *bugs.ClusterMetrics, issuePriority string) string {
	priorityIndex := g.indexOfPriority(issuePriority)
	if priorityIndex >= len(g.monorailCfg.Priorities) {
		// Unknown priority - it should be one of the configured priorities.
		return ""
	}

	thresholdsMet := g.monorailCfg.Priorities[priorityIndex].Thresholds
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
func (g *Generator) priorityDecreaseJustification(oldPriorityIndex, newPriorityIndex int) string {
	if newPriorityIndex <= oldPriorityIndex {
		// Priority did not change or increased.
		return ""
	}

	// Priority decreased.
	// To justify the decrease, it is sufficient to explain why we could no
	// longer meet the criteria for the next-higher priority.
	hysteresisPerc := g.monorailCfg.PriorityHysteresisPercent

	// The next-higher priority level that we failed to meet.
	failedToMeetThreshold := g.monorailCfg.Priorities[newPriorityIndex-1].Thresholds
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
func (g *Generator) priorityIncreaseJustification(metrics *bugs.ClusterMetrics, oldPriorityIndex, newPriorityIndex int) string {
	if newPriorityIndex >= oldPriorityIndex {
		// Priority did not change or decreased.
		return ""
	}

	// Priority increased.
	// To justify the increase, we must show that we met the criteria for
	// each successively higher priority level.
	hysteresisPerc := g.monorailCfg.PriorityHysteresisPercent

	// Visit priorities in increasing priority order.
	var thresholdsMet [][]*configpb.ImpactMetricThreshold
	for i := oldPriorityIndex - 1; i >= newPriorityIndex; i-- {
		metThreshold := g.monorailCfg.Priorities[i].Thresholds
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
func (g *Generator) clusterPriority(metrics *bugs.ClusterMetrics) string {
	return g.clusterPriorityWithInflatedThresholds(metrics, 0)
}

// clusterPriorityWithInflatedThresholds returns the desired priority of the bug,
// if thresholds are inflated or deflated with the given percentage.
//
// See bugs.InflateThreshold for the interpretation of inflationPercent.
func (g *Generator) clusterPriorityWithInflatedThresholds(metrics *bugs.ClusterMetrics, inflationPercent int64) string {
	// Default to using the lowest priority.
	priority := g.monorailCfg.Priorities[len(g.monorailCfg.Priorities)-1]
	for i := len(g.monorailCfg.Priorities) - 2; i >= 0; i-- {
		p := g.monorailCfg.Priorities[i]
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
func (g *Generator) clusterResolved(metrics *bugs.ClusterMetrics) bool {
	lowestPriority := g.monorailCfg.Priorities[len(g.monorailCfg.Priorities)-1]
	return !metrics.MeetsAnyOfThresholds(lowestPriority.Thresholds)
}
