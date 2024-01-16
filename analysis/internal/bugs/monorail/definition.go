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
	"strings"
	"time"

	"google.golang.org/genproto/protobuf/field_mask"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/analysis/internal/bugs"
	mpb "go.chromium.org/luci/analysis/internal/bugs/monorail/api_proto"
	bugspb "go.chromium.org/luci/analysis/internal/bugs/proto"
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
	// The policy applyer instance used to apply bug management policy.
	policyApplyer bugs.PolicyApplyer
}

// NewGenerator initialises a new Generator.
func NewGenerator(uiBaseURL, project string, projectCfg *configpb.ProjectConfig) (*RequestGenerator, error) {
	if projectCfg.BugManagement.GetMonorail() == nil {
		return nil, errors.Reason("monorail configuration not set").Err()
	}

	// Monorail projects have a floor priority of P3.
	policyApplyer, err := bugs.NewPolicyApplyer(projectCfg.BugManagement.GetPolicies(), configpb.BuganizerPriority_P3)
	if err != nil {
		return nil, err
	}

	return &RequestGenerator{
		uiBaseURL:     uiBaseURL,
		project:       project,
		monorailCfg:   projectCfg.BugManagement.Monorail,
		policyApplyer: policyApplyer,
	}, nil
}

func (rg *RequestGenerator) PrepareNew(ruleID string, activePolicyIDs map[bugs.PolicyID]struct{}, description *clustering.ClusterDescription,
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

	ruleLink := bugs.RuleURL(rg.uiBaseURL, rg.project, ruleID)

	return &mpb.MakeIssueRequest{
		Parent: fmt.Sprintf("projects/%s", rg.monorailCfg.Project),
		Issue:  issue,
		Description: rg.policyApplyer.NewIssueDescription(
			description, activePolicyIDs, rg.uiBaseURL, ruleLink),
		NotifyType: mpb.NotifyType_EMAIL,
	}, nil
}

// linkToRuleComment returns a comment that links the user to the failure
// association rule in LUCI Analysis.
//
// ruleID is the LUCI Analysis Rule ID.
func (rg *RequestGenerator) linkToRuleComment(ruleID string) string {
	return fmt.Sprintf(bugs.LinkTemplate, bugs.RuleURL(rg.uiBaseURL, rg.project, ruleID))
}

// PrepareRuleAssociatedComment prepares a request that notifies the bug
// it is associated with failures in LUCI Analysis.
func (rg *RequestGenerator) PrepareRuleAssociatedComment(ruleID, bugID string) (*mpb.ModifyIssuesRequest, error) {
	issueName, err := ToMonorailIssueName(bugID)
	if err != nil {
		return nil, err
	}

	ruleURL := bugs.RuleURL(rg.uiBaseURL, rg.project, ruleID)

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
		CommentContent: bugs.RuleAssociatedCommentary(ruleURL).ToComment(),
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
func (rg *RequestGenerator) PreparePolicyActivatedComment(ruleID, bugID string, policyID bugs.PolicyID) (*mpb.ModifyIssuesRequest, error) {
	templateInput := bugs.TemplateInput{
		RuleURL: bugs.RuleURL(rg.uiBaseURL, rg.project, ruleID),
		BugID:   bugs.NewTemplateBugID(bugs.BugID{System: bugs.MonorailSystem, ID: bugID}),
	}
	comment, err := rg.policyApplyer.PolicyActivatedComment(policyID, rg.uiBaseURL, templateInput)
	if err != nil {
		return nil, err
	}

	labelsToAdd := rg.labelsForPolicy(policyID)

	if comment == "" && len(labelsToAdd) == 0 {
		// Policy has not specified a comment to post or label to add. This is fine.
		return nil, nil
	}

	name, err := ToMonorailIssueName(bugID)
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
	if len(labelsToAdd) > 0 {
		// Note: labels specified here are interpreted by monorail
		// as requests to add labels. The absence of a label already
		// on the issue does not results in its deletion.
		//
		// To remove a label, we must specify it on delta.LabelsRemove.
		delta.Issue.Labels = labelsToAdd
		delta.UpdateMask.Paths = append(delta.UpdateMask.Paths, "labels")
	}

	req := &mpb.ModifyIssuesRequest{
		Deltas: []*mpb.IssueDelta{
			delta,
		},
	}
	if comment != "" {
		req.CommentContent = comment
		req.NotifyType = mpb.NotifyType_EMAIL
	}
	return req, nil
}

// UpdateDuplicateSource updates the source bug of a (source, destination)
// duplicate bug pair, after LUCI Analysis has attempted to merge their
// failure association rules.
func (rg *RequestGenerator) UpdateDuplicateSource(bugID, errorMessage, sourceRuleID, destinationRuleID string) (*mpb.ModifyIssuesRequest, error) {
	name, err := ToMonorailIssueName(bugID)
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
		comment = strings.Join([]string{errorMessage, rg.linkToRuleComment(sourceRuleID)}, "\n\n")
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

type MakeUpdateOptions struct {
	// The LUCI Analysis Rule ID.
	RuleID string
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

	commentary.Footers = append(commentary.Footers, rg.linkToRuleComment(options.RuleID))

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

func issueVerified(issue *mpb.Issue) bool {
	return issue.Status.Status == VerifiedStatus
}

func (rg *RequestGenerator) labelsForPolicy(policyID bugs.PolicyID) []*mpb.Issue_LabelValue {
	policy := rg.policyApplyer.PolicyByID(policyID)
	if policy == nil {
		// Policy no longer configured.
		return nil
	}
	if policy.BugTemplate.GetMonorail() == nil {
		// Monorail bug template not configured.
		return nil
	}

	// Config validation ensures that the labels specified
	// on the policy are distinct values.
	var result []*mpb.Issue_LabelValue
	for _, label := range policy.BugTemplate.Monorail.Labels {
		result = append(result, &mpb.Issue_LabelValue{
			Label: label,
		})
	}
	return result
}
