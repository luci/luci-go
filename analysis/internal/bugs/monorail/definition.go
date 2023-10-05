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
	bugspb "go.chromium.org/luci/analysis/internal/bugs/proto"
	"go.chromium.org/luci/analysis/internal/clustering"
	configpb "go.chromium.org/luci/analysis/proto/config"
	"go.chromium.org/luci/common/errors"
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

// RequestGenerator provides access to methods to generate a new bug and/or bug
// updates for a cluster.
type RequestGenerator struct {
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
	// The policy applyer instance used to apply bug management policy.
	policyApplyer bugs.PolicyApplyer
}

// NewGenerator initialises a new Generator.
func NewGenerator(uiBaseURL, project string, projectCfg *configpb.ProjectConfig) (*RequestGenerator, error) {
	if len(projectCfg.Monorail.Priorities) == 0 {
		return nil, fmt.Errorf("invalid configuration for monorail project %q; no monorail priorities configured", projectCfg.Monorail.Project)
	}
	// Monorail projects have a floor priority of P3.
	policyApplyer, err := bugs.NewPolicyApplyer(projectCfg.BugManagement.GetPolicies(), configpb.BuganizerPriority_P3)
	if err != nil {
		return nil, err
	}

	return &RequestGenerator{
		uiBaseURL:           uiBaseURL,
		project:             project,
		monorailCfg:         projectCfg.Monorail,
		bugFilingThresholds: projectCfg.BugFilingThresholds,
		policyApplyer:       policyApplyer,
	}, nil
}

func (rg *RequestGenerator) PrepareNew(activePolicyIDs map[bugs.PolicyID]struct{}, description *clustering.ClusterDescription,
	components []string) (*mpb.MakeIssueRequest, error) {

	priority, verified := rg.policyApplyer.RecommendedPriorityAndVerified(activePolicyIDs)
	if verified {
		return nil, errors.Reason("issue is recommended to be verified from time of creation; are no policies active?").Err()
	}
	issue := &mpb.Issue{
		Summary: bugs.GenerateBugSummary(description.Title),
		State:   mpb.IssueContentState_ACTIVE,
		Status:  &mpb.Issue_StatusValue{Status: UntriagedStatus},
		FieldValues: []*mpb.FieldValue{
			{
				Field: rg.priorityFieldName(),
				Value: toMonorailPriority(priority),
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

	return &mpb.MakeIssueRequest{
		Parent: fmt.Sprintf("projects/%s", rg.monorailCfg.Project),
		Issue:  issue,
		// Do not include the link to the rule in monorail initial comments,
		// as we will post it in a follow-up comment.
		Description: rg.policyApplyer.NewIssueDescription(
			description, activePolicyIDs, rg.uiBaseURL, "" /* ruleLink */),
		NotifyType: mpb.NotifyType_EMAIL,
	}, nil
}

// PrepareNewLegacy prepares a new bug from the given cluster. Title and description
// are the cluster-specific bug title and description.
func (rg *RequestGenerator) PrepareNewLegacy(metrics bugs.ClusterMetrics, description *clustering.ClusterDescription, components []string) *mpb.MakeIssueRequest {
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
func (rg *RequestGenerator) linkToRuleComment(bugName string) string {
	return fmt.Sprintf(bugs.LinkTemplate, bugs.RuleForMonorailBugURL(rg.uiBaseURL, bugName))
}

// PrepareLinkComment prepares a request that adds links to LUCI Analysis to
// a monorail bug.
func (rg *RequestGenerator) PrepareLinkComment(bugID string) (*mpb.ModifyIssuesRequest, error) {
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

// SortPolicyIDsByPriorityDescending sorts policy IDs in descending
// priority order (i.e. P0 policies first, then P1, then P2, ...).
func (rg *RequestGenerator) SortPolicyIDsByPriorityDescending(policyIDs map[bugs.PolicyID]struct{}) []bugs.PolicyID {
	return rg.policyApplyer.SortPolicyIDsByPriorityDescending(policyIDs)
}

// PreparePolicyActivatedComment prepares a request that notifies a bug that a policy
// has activated for the first time.
// This method returns nil if the policy has not specified any comment to post.
func (rg *RequestGenerator) PreparePolicyActivatedComment(bugID string, policyID bugs.PolicyID) (*mpb.ModifyIssuesRequest, error) {
	templateInput := bugs.TemplateInput{
		RuleURL: bugs.RuleForMonorailBugURL(rg.uiBaseURL, bugID),
		BugID:   bugs.NewTemplateBugID(bugs.BugID{System: bugs.MonorailSystem, ID: bugID}),
	}
	comment, err := rg.policyApplyer.PolicyActivatedComment(policyID, rg.uiBaseURL, templateInput)
	if err != nil {
		return nil, err
	}

	if comment == "" {
		// Policy has not specified a comment to post. This is fine.
		return nil, nil
	}

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

	req := &mpb.ModifyIssuesRequest{
		Deltas: []*mpb.IssueDelta{
			delta,
		},
		NotifyType:     mpb.NotifyType_EMAIL,
		CommentContent: comment,
	}
	return req, nil
}

// UpdateDuplicateSource updates the source bug of a (source, destination)
// duplicate bug pair, after LUCI Analysis has attempted to merge their
// failure association rules.
func (rg *RequestGenerator) UpdateDuplicateSource(bugID, errorMessage, destinationRuleID string) (*mpb.ModifyIssuesRequest, error) {
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
func (rg *RequestGenerator) UpdateDuplicateDestination(bugName string) (*mpb.ModifyIssuesRequest, error) {
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

func (rg *RequestGenerator) priorityFieldName() string {
	return fmt.Sprintf("projects/%s/fieldDefs/%v", rg.monorailCfg.Project, rg.monorailCfg.PriorityFieldId)
}

// NeedsPriorityOrVerifiedUpdate returns whether the bug priority and/or verified
// status needs to be updated.
func (rg *RequestGenerator) NeedsPriorityOrVerifiedUpdate(bms *bugspb.BugManagementState, issue *mpb.Issue, isManagingBugPriority bool) (bool, error) {
	priority, err := rg.IssuePriority(issue)
	if err != nil {
		return false, nil
	}

	opts := bugs.BugOptions{
		State:              bms,
		IsManagingPriority: isManagingBugPriority,
		ExistingPriority:   priority,
		ExistingVerified:   issueVerified(issue),
	}
	needsUpdate := rg.policyApplyer.NeedsPriorityOrVerifiedUpdate(opts)
	return needsUpdate, nil
}

// IssuePriority returns the priority of the given issue.
func (rg *RequestGenerator) IssuePriority(issue *mpb.Issue) (configpb.BuganizerPriority, error) {
	priorityFieldName := rg.priorityFieldName()
	var fieldValue string
	for _, fv := range issue.FieldValues {
		if fv.Field == priorityFieldName {
			fieldValue = fv.Value
			break
		}
	}
	return fromMonorailPriority(fieldValue)
}

func toMonorailPriority(priority configpb.BuganizerPriority) string {
	switch priority {
	case configpb.BuganizerPriority_P0:
		return "0"
	case configpb.BuganizerPriority_P1:
		return "1"
	case configpb.BuganizerPriority_P2:
		return "2"
	case configpb.BuganizerPriority_P3:
		return "3"
	}
	return ""
}

func fromMonorailPriority(priority string) (configpb.BuganizerPriority, error) {
	switch priority {
	case "0":
		return configpb.BuganizerPriority_P0, nil
	case "1":
		return configpb.BuganizerPriority_P1, nil
	case "2":
		return configpb.BuganizerPriority_P2, nil
	case "3":
		return configpb.BuganizerPriority_P3, nil
	}
	// Cannot determine issue priority
	return configpb.BuganizerPriority_BUGANIZER_PRIORITY_UNSPECIFIED,
		errors.Reason("unrecognised monorail priority %q", priority).Err()
}

// NeedsUpdateLegacy determines if the bug for the given cluster needs to be updated.
func (rg *RequestGenerator) NeedsUpdateLegacy(metrics bugs.ClusterMetrics, issue *mpb.Issue, isManagingBugPriority bool) bool {
	// Cases that a bug may be updated follow.
	switch {
	case !rg.isCompatibleWithVerified(metrics, issueVerified(issue)):
		return true
	case isManagingBugPriority &&
		!issueVerified(issue) &&
		!rg.isCompatibleWithPriority(metrics, rg.IssuePriorityLegacy(issue)):
		// The priority has changed on a cluster which is not verified as fixed
		// and the user isn't manually controlling the priority.
		return true
	default:
		return false
	}
}

type MakeUpdateOptions struct {
	// The bug management state.
	BugManagementState *bugspb.BugManagementState
	// The issue to update.
	Issue *mpb.Issue
	// Indicates whether the rule is managing bug priority or not.
	// Use the value on the rule; do not yet set it to false if
	// HasManuallySetPriority is true.
	IsManagingBugPriority bool
	// Whether the user has manually taken control of the bug priority.
	HasManuallySetPriority bool
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

// MakeUpdateResult is the result of MakePriorityOrVerifiedUpdate.
type MakeUpdateResult struct {
	// The created request to modify an issue
	request *mpb.ModifyIssuesRequest

	// Whether isManagingBugPriority should be disabled on the rule.
	// This is determined by checking if the last time the flag was turned on
	// was after the last time a user updated the priority manually.
	disableBugPriorityUpdates bool
}

// MakePriorityOrVerifiedUpdate prepares an update for the bug with the given
// bug management state.
// **Must** ONLY be called if NeedsPriorityOrVerifiedUpdate(...) returns true.
func (rg *RequestGenerator) MakePriorityOrVerifiedUpdate(options MakeUpdateOptions) (MakeUpdateResult, error) {

	priority, err := rg.IssuePriority(options.Issue)
	if err != nil {
		return MakeUpdateResult{}, nil
	}
	opts := bugs.BugOptions{
		State:              options.BugManagementState,
		IsManagingPriority: options.IsManagingBugPriority && !options.HasManuallySetPriority,
		ExistingPriority:   priority,
		ExistingVerified:   issueVerified(options.Issue),
	}

	change, err := rg.policyApplyer.PreparePriorityAndVerifiedChange(opts, rg.uiBaseURL)
	if err != nil {
		return MakeUpdateResult{}, nil
	}

	delta := &mpb.IssueDelta{
		Issue: &mpb.Issue{
			Name: options.Issue.Name,
		},
		UpdateMask: &field_mask.FieldMask{
			Paths: []string{},
		},
	}
	var notify bool
	if change.UpdatePriority {
		delta.Issue.FieldValues = []*mpb.FieldValue{
			{
				Field: rg.priorityFieldName(),
				Value: toMonorailPriority(change.Priority),
			},
		}
		delta.UpdateMask.Paths = append(delta.UpdateMask.Paths, "field_values")
		// Notify if the new priority is higher than the old priority,
		// e.g. we are going from P2 to P1.
		notify = change.Priority < priority
	}
	if change.UpdateVerified {
		var status string
		if change.ShouldBeVerified {
			status = VerifiedStatus
		} else {
			if options.Issue.GetOwner().GetUser() != "" {
				status = AssignedStatus
			} else {
				status = UntriagedStatus
			}
		}
		delta.Issue.Status = &mpb.Issue_StatusValue{Status: status}
		delta.UpdateMask.Paths = append(delta.UpdateMask.Paths, "status")
		notify = true
	}

	var commentary bugs.Commentary
	if change.UpdateVerified || change.UpdatePriority {
		commentary = change.Justification
	}
	if options.HasManuallySetPriority {
		commentary = bugs.MergeCommentary(commentary, bugs.ManualPriorityUpdateCommentary())
	}

	bugName, err := fromMonorailIssueName(options.Issue.Name)
	if err != nil {
		// This should never happen. It would mean monorail is feeding us
		// invalid data.
		return MakeUpdateResult{}, errors.Reason("invalid monorail issue name: %q", options.Issue.Name).Err()
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
		disableBugPriorityUpdates: options.HasManuallySetPriority,
	}, nil
}

// MakeUpdateLegacy prepares an updated for the bug associated with a given cluster.
// Must ONLY be called if NeedsUpdate(...) returns true.
func (rg *RequestGenerator) MakeUpdateLegacy(options MakeUpdateLegacyOptions) MakeUpdateResult {
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
		!rg.isCompatibleWithPriority(options.metrics, rg.IssuePriorityLegacy(options.issue)) {
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
			notify = notify || rg.isHigherPriority(rg.clusterPriority(options.metrics), rg.IssuePriorityLegacy(options.issue))
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

func (rg *RequestGenerator) prepareBugVerifiedUpdate(metrics bugs.ClusterMetrics, issue *mpb.Issue, update *mpb.IssueDelta) bugs.Commentary {
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

func (rg *RequestGenerator) preparePriorityUpdate(metrics bugs.ClusterMetrics, issue *mpb.Issue, update *mpb.IssueDelta) bugs.Commentary {
	newPriority := rg.clusterPriority(metrics)

	update.Issue.FieldValues = []*mpb.FieldValue{
		{
			Field: rg.priorityFieldName(),
			Value: newPriority,
		},
	}
	update.UpdateMask.Paths = append(update.UpdateMask.Paths, "field_values")

	oldPriority := rg.IssuePriorityLegacy(issue)
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

// IssuePriorityLegacy returns the priority of the given issue.
func (rg *RequestGenerator) IssuePriorityLegacy(issue *mpb.Issue) string {
	priorityFieldName := rg.priorityFieldName()
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
func (rg *RequestGenerator) isHigherPriority(p1 string, p2 string) bool {
	i1 := rg.indexOfPriority(p1)
	i2 := rg.indexOfPriority(p2)
	// Priorities are configured from highest to lowest, so higher priorities
	// have lower indexes.
	return i1 < i2
}

func (rg *RequestGenerator) indexOfPriority(priority string) int {
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
func (rg *RequestGenerator) isCompatibleWithVerified(metrics bugs.ClusterMetrics, verified bool) bool {
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
func (rg *RequestGenerator) isCompatibleWithPriority(metrics bugs.ClusterMetrics, issuePriority string) bool {
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
func (rg *RequestGenerator) priorityComment(metrics bugs.ClusterMetrics, issuePriority string) string {
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
func (rg *RequestGenerator) priorityDecreaseJustification(oldPriorityIndex, newPriorityIndex int) string {
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
func (rg *RequestGenerator) priorityIncreaseJustification(metrics bugs.ClusterMetrics, oldPriorityIndex, newPriorityIndex int) string {
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
func (rg *RequestGenerator) clusterPriority(metrics bugs.ClusterMetrics) string {
	return rg.clusterPriorityWithInflatedThresholds(metrics, 0)
}

// clusterPriorityWithInflatedThresholds returns the desired priority of the bug,
// if thresholds are inflated or deflated with the given percentage.
//
// See bugs.InflateThreshold for the interpretation of inflationPercent.
func (rg *RequestGenerator) clusterPriorityWithInflatedThresholds(metrics bugs.ClusterMetrics, inflationPercent int64) string {
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
func (rg *RequestGenerator) clusterResolved(metrics bugs.ClusterMetrics) bool {
	lowestPriority := rg.monorailCfg.Priorities[len(rg.monorailCfg.Priorities)-1]
	return !metrics.MeetsAnyOfThresholds(lowestPriority.Thresholds)
}
