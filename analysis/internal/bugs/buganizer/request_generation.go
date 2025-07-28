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
	"fmt"
	"strconv"
	"strings"

	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/third_party/google.golang.org/genproto/googleapis/devtools/issuetracker/v1"

	"go.chromium.org/luci/analysis/internal/bugs"
	bugspb "go.chromium.org/luci/analysis/internal/bugs/proto"
	"go.chromium.org/luci/analysis/internal/clustering"
	configpb "go.chromium.org/luci/analysis/proto/config"
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

// RequestGenerator generates new bugs or prepares existing ones
// for updates.
type RequestGenerator struct {
	// The issuetracker client that will be used to make RPCs to Buganizer.
	client Client
	// The LUCI project for which we are generating bug updates. This
	// is distinct from the Buganizer project.
	project string
	// The UI Base URL, e.g. "https://luci-milo.appspot.com"
	uiBaseURL string
	// The email address the service uses to authenticate to Buganizer.
	selfEmail string
	// The Buganizer config of the LUCI project config.
	buganizerCfg *configpb.BuganizerProject
	// The policy applyer instance used to apply bug management policy.
	policyApplyer bugs.PolicyApplyer
}

// NewRequestGenerator initializes a new buganizer request generator.
func NewRequestGenerator(
	client Client,
	project, uiBaseURL, selfEmail string,
	projectCfg *configpb.ProjectConfig) (*RequestGenerator, error) {
	if projectCfg.BugManagement.GetBuganizer() == nil {
		return nil, errors.New("buganizer configuration not set")
	}

	// Buganizer supports all priority levels P4 and above.
	policyApplyer, err := bugs.NewPolicyApplyer(projectCfg.BugManagement.GetPolicies(), configpb.BuganizerPriority_P4)
	if err != nil {
		return nil, errors.Fmt("create policy applyer: %w", err)
	}

	return &RequestGenerator{
		client:        client,
		uiBaseURL:     uiBaseURL,
		selfEmail:     selfEmail,
		project:       project,
		buganizerCfg:  projectCfg.BugManagement.Buganizer,
		policyApplyer: policyApplyer,
	}, nil
}

// PrepareNew generates a CreateIssueRequest for a new issue.
func (rg *RequestGenerator) PrepareNew(description *clustering.ClusterDescription, activePolicyIDs map[bugs.PolicyID]struct{},
	ruleID string, component *issuetracker.Component) (*issuetracker.CreateIssueRequest, error) {
	priority, verified := rg.policyApplyer.RecommendedPriorityAndVerified(activePolicyIDs)
	if verified {
		return nil, errors.New("issue is recommended to be verified from time of creation; are no policies active?")
	}

	ruleLink := bugs.RuleURL(rg.uiBaseURL, rg.project, ruleID)

	accessLimit := issuetracker.IssueAccessLimit_LIMIT_VIEW_TRUSTED
	if rg.buganizerCfg.FileWithoutLimitViewTrusted {
		accessLimit = issuetracker.IssueAccessLimit_LIMIT_NONE
	}
	// Issues under internal components cannot be viewed by the public regardless
	// of the access limit setting on the issues themselves.
	// Attaching `LIMIT_VIEW_TRUSTED` to those issues causes automation tools,
	// including LUCI Analysis itself, to lose access to those issues.
	if component.AccessLimit.AccessLevel == issuetracker.AccessLimit_INTERNAL {
		accessLimit = issuetracker.IssueAccessLimit_LIMIT_NONE
	}

	issue := &issuetracker.Issue{
		IssueState: &issuetracker.IssueState{
			ComponentId: component.ComponentId,
			Type:        issuetracker.Issue_BUG,
			Status:      issuetracker.Issue_NEW,
			Priority:    toBuganizerPriority(priority),
			Severity:    issuetracker.Issue_S2,
			Title:       bugs.GenerateBugSummary(description.Title),
			AccessLimit: &issuetracker.IssueAccessLimit{
				AccessLevel: accessLimit,
			},
		},
		IssueComment: &issuetracker.IssueComment{
			Comment: rg.policyApplyer.NewIssueDescription(
				description, activePolicyIDs, rg.uiBaseURL, ruleLink),
		},
	}

	return &issuetracker.CreateIssueRequest{
		Issue: issue,
		TemplateOptions: &issuetracker.CreateIssueRequest_TemplateOptions{
			ApplyTemplate: true,
		},
	}, nil
}

// linkToRuleComment returns a comment that links the user to the failure
// association rule in LUCI Analysis.
//
// ruleID is the LUCI Analysis Rule ID.
func (rg *RequestGenerator) linkToRuleComment(ruleID string) string {
	ruleLink := bugs.RuleURL(rg.uiBaseURL, rg.project, ruleID)
	return fmt.Sprintf(bugs.LinkTemplate, ruleLink)
}

// componentFallbackComment returns a comment that explains that a bug
// was filed in the fallback component.
//
// issueId is the Buganizer issueId.
func (rg *RequestGenerator) componentFallbackComment(componentID int64, reason ComponentFallbackReason) string {
	switch reason {
	case FallbackReason_NoPermission:
		return fmt.Sprintf(bugs.NoPermissionTemplate, componentID)
	case FallbackReason_ComponentArchived:
		return fmt.Sprintf(bugs.ComponentArchivedTemplate, componentID)
	default:
		panic(fmt.Sprintf("invalid fallback reason: %v", reason))
	}
}

type ComponentFallbackReason int

const (
	FallbackReason_None ComponentFallbackReason = iota
	FallbackReason_NoPermission
	FallbackReason_ComponentArchived
)

// PrepareComponentFallbackComment prepares a request that adds a comment
// explaining why LUCI Analysis did not use the originally requested bug
// filing component but used the fallback component instead.
func (rg *RequestGenerator) PrepareComponentFallbackComment(issueID, componentID int64, reason ComponentFallbackReason) *issuetracker.CreateIssueCommentRequest {
	return &issuetracker.CreateIssueCommentRequest{
		IssueId: issueID,
		Comment: &issuetracker.IssueComment{
			Comment: rg.componentFallbackComment(componentID, reason),
		},
	}
}

// PrepareRuleAssociatedComment prepares a request that notifies the bug
// it is associated with failures in LUCI Analysis.
func (rg *RequestGenerator) PrepareRuleAssociatedComment(ruleID string, issueID int64) (*issuetracker.CreateIssueCommentRequest, error) {
	ruleURL := bugs.RuleURL(rg.uiBaseURL, rg.project, ruleID)

	return &issuetracker.CreateIssueCommentRequest{
		IssueId: issueID,
		Comment: &issuetracker.IssueComment{
			Comment: bugs.RuleAssociatedCommentary(ruleURL).ToComment(),
		},
		SignificanceOverride: issuetracker.EditSignificance_MINOR,
	}, nil
}

// SortPolicyIDsByPriorityDescending sorts policy IDs in descending
// priority order (i.e. P0 policies first, then P1, then P2, ...).
func (rg *RequestGenerator) SortPolicyIDsByPriorityDescending(policyIDs map[bugs.PolicyID]struct{}) []bugs.PolicyID {
	return rg.policyApplyer.SortPolicyIDsByPriorityDescending(policyIDs)
}

// PreparePolicyActivatedComment prepares a request that notifies a bug that a policy
// has activated for the first time.
// If the policy has not specified a comment to post, this method returns nil.
func (rg *RequestGenerator) PreparePolicyActivatedComment(ruleID string, issueID int64, policyID bugs.PolicyID) (*issuetracker.CreateIssueCommentRequest, error) {
	templateInput := bugs.TemplateInput{
		RuleURL: bugs.RuleURL(rg.uiBaseURL, rg.project, ruleID),
		BugID:   bugs.NewTemplateBugID(bugs.BugID{System: bugs.BuganizerSystem, ID: strconv.FormatInt(issueID, 10)}),
	}
	comment, err := rg.policyApplyer.PolicyActivatedComment(policyID, rg.uiBaseURL, templateInput)
	if err != nil {
		return nil, err
	}
	if comment == "" {
		return nil, nil
	}
	return &issuetracker.CreateIssueCommentRequest{
		IssueId: issueID,
		Comment: &issuetracker.IssueComment{
			Comment: comment,
		},
	}, nil
}

// UpdateDuplicateSource updates the source bug of a (source, destination)
// duplicate bug pair, after LUCI Analysis has attempted to merge their
// failure association rules.
func (rg *RequestGenerator) UpdateDuplicateSource(issueID int64, errorMessage, sourceRuleID, destinationRuleID string, isAssigned bool) *issuetracker.ModifyIssueRequest {
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
			Comment: strings.Join([]string{errorMessage, rg.linkToRuleComment(sourceRuleID)}, "\n\n"),
		}
	} else {
		ruleLink := bugs.RuleURL(rg.uiBaseURL, rg.project, destinationRuleID)
		updateRequest.IssueComment = &issuetracker.IssueComment{
			Comment: fmt.Sprintf(bugs.SourceBugRuleUpdatedTemplate, ruleLink),
		}
	}
	return updateRequest
}

// NeedsPriorityOrVerifiedUpdate returns whether the bug priority and/or verified
// status needs to be updated.
func (rg *RequestGenerator) NeedsPriorityOrVerifiedUpdate(bms *bugspb.BugManagementState,
	issue *issuetracker.Issue,
	isManagingBugPriority bool) bool {
	opts := bugs.BugOptions{
		State:              bms,
		IsManagingPriority: isManagingBugPriority,
		ExistingPriority:   fromBuganizerPriority(issue.IssueState.Priority),
		ExistingVerified:   issue.IssueState.Status == issuetracker.Issue_VERIFIED,
	}
	return rg.policyApplyer.NeedsPriorityOrVerifiedUpdate(opts)
}

func toBuganizerPriority(priority configpb.BuganizerPriority) issuetracker.Issue_Priority {
	return configPriorityToIssueTrackerPriority[priority]
}

func fromBuganizerPriority(priority issuetracker.Issue_Priority) configpb.BuganizerPriority {
	for configPri, issuePri := range configPriorityToIssueTrackerPriority {
		if issuePri == priority {
			return configPri
		}
	}
	panic(fmt.Sprintf("fromBuganizerPriority - should be unreachable (priority: %v)", priority))
}

type MakeUpdateOptions struct {
	// The identifier of the rule making the update.
	RuleID string
	// The bug management state.
	BugManagementState *bugspb.BugManagementState
	// The Issue to update.
	Issue *issuetracker.Issue
	// Indicates whether the rule is managing bug priority or not.
	// Use the value on the rule; do not yet set it to false if
	// HasManuallySetPriority is true.
	IsManagingBugPriority bool
	// Whether the user has manually taken control of the bug priority.
	HasManuallySetPriority bool
}

// MakeUpdateResult is the result of MakePriorityOrVerifiedUpdate.
type MakeUpdateResult struct {
	// The generated request.
	request *issuetracker.ModifyIssueRequest
	// disablePriorityUpdates is set when the user has manually
	// made a priority update since the last time automatic
	// priority updates were enabled
	disablePriorityUpdates bool
}

// MakePriorityOrVerifiedUpdate prepares a priority and/or verified update for the
// bug with the given bug management state.
// **Must** ONLY be called if NeedsPriorityOrVerifiedUpdate(...) returns true.
func (rg *RequestGenerator) MakePriorityOrVerifiedUpdate(options MakeUpdateOptions) (MakeUpdateResult, error) {
	opts := bugs.BugOptions{
		State:              options.BugManagementState,
		IsManagingPriority: options.IsManagingBugPriority && !options.HasManuallySetPriority,
		ExistingPriority:   fromBuganizerPriority(options.Issue.IssueState.Priority),
		ExistingVerified:   options.Issue.IssueState.Status == issuetracker.Issue_VERIFIED,
	}

	change, err := rg.policyApplyer.PreparePriorityAndVerifiedChange(opts, rg.uiBaseURL)
	if err != nil {
		return MakeUpdateResult{}, errors.Fmt("prepare change: %w", err)
	}

	request := &issuetracker.ModifyIssueRequest{
		IssueId:      options.Issue.IssueId,
		AddMask:      &fieldmaskpb.FieldMask{},
		Add:          &issuetracker.IssueState{},
		RemoveMask:   &fieldmaskpb.FieldMask{},
		Remove:       &issuetracker.IssueState{},
		IssueComment: &issuetracker.IssueComment{},
	}

	if change.UpdatePriority {
		request.AddMask.Paths = append(request.AddMask.Paths, "priority")
		request.Add.Priority = toBuganizerPriority(change.Priority)
	}
	if change.UpdateVerified {
		if change.ShouldBeVerified {
			// Mark LUCI Analysis the verifier.
			request.Add.Verifier = &issuetracker.User{
				EmailAddress: rg.selfEmail,
			}
			request.AddMask.Paths = append(request.AddMask.Paths, "verifier")

			if options.Issue.IssueState.Assignee == nil {
				// Make LUCI Analysis the assignee if there is no assignee.
				request.Add.Assignee = &issuetracker.User{
					EmailAddress: rg.selfEmail,
				}
				request.AddMask.Paths = append(request.AddMask.Paths, "assignee")
			}

			request.Add.Status = issuetracker.Issue_VERIFIED
			request.AddMask.Paths = append(request.AddMask.Paths, "status")
		} else {
			var status issuetracker.Issue_Status

			if options.Issue.IssueState.Assignee == nil {
				status = issuetracker.Issue_NEW
			} else {
				if options.Issue.IssueState.Assignee.EmailAddress == rg.selfEmail {
					// In case the current assignee is LUCI Analysis itself
					// from an earlier bug verification.
					status = issuetracker.Issue_NEW

					request.Remove.Assignee = &issuetracker.User{}
					request.RemoveMask.Paths = append(request.RemoveMask.Paths, "assignee")
				} else {
					status = issuetracker.Issue_ASSIGNED
				}
			}
			request.Add.Status = status
			request.AddMask.Paths = append(request.AddMask.Paths, "status")
		}
	}

	var result MakeUpdateResult
	var commentary bugs.Commentary
	if change.UpdatePriority || change.UpdateVerified {
		commentary = change.Justification
	}
	if options.HasManuallySetPriority {
		commentary = bugs.MergeCommentary(commentary, bugs.ManualPriorityUpdateCommentary())
		result.disablePriorityUpdates = true
	}
	commentary.Footers = append(commentary.Footers, rg.linkToRuleComment(options.RuleID))

	request.IssueComment = &issuetracker.IssueComment{
		IssueId: options.Issue.IssueId,
		Comment: commentary.ToComment(),
	}
	result.request = request
	return result, nil
}

func (rg *RequestGenerator) ExpectedHotlistIDs(activePolicyIDs map[bugs.PolicyID]struct{}) map[int64]struct{} {
	expectedHotlistIDs := make(map[int64]struct{})

	policies := rg.policyApplyer.PoliciesByIDs(activePolicyIDs)
	for _, policy := range policies {
		buganizerTemplate := policy.BugTemplate.GetBuganizer()
		if buganizerTemplate != nil {
			for _, id := range buganizerTemplate.Hotlists {
				expectedHotlistIDs[id] = struct{}{}
			}
		}
	}
	return expectedHotlistIDs
}

// PrepareHotlistInsertions returns the CreateHotlistEntry requests
// necessary to insert the issue in the hotlists specified by its
// bug managment policies.
func PrepareHotlistInsertions(hotlistIDs map[int64]struct{}, issueID int64) []*issuetracker.CreateHotlistEntryRequest {
	var result []*issuetracker.CreateHotlistEntryRequest
	for hotlistID := range hotlistIDs {
		request := &issuetracker.CreateHotlistEntryRequest{
			HotlistId: hotlistID,
			HotlistEntry: &issuetracker.HotlistEntry{
				IssueId: issueID,
			},
		}
		result = append(result, request)
	}
	return result
}
