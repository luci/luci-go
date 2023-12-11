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

// Package monorail contains monorail-specific logic for
// creating and updating bugs.
package monorail

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/analysis/internal/bugs"
	mpb "go.chromium.org/luci/analysis/internal/bugs/monorail/api_proto"
	configpb "go.chromium.org/luci/analysis/proto/config"
)

// monorailRe matches monorail issue names, like
// "monorail/{monorail_project}/{numeric_id}".
var monorailRe = regexp.MustCompile(`^projects/([a-z0-9\-_]+)/issues/([0-9]+)$`)

// componentRE matches valid full monorail component names.
var componentRE = regexp.MustCompile(`^[a-zA-Z]([-_]?[a-zA-Z0-9])+(\>[a-zA-Z]([-_]?[a-zA-Z0-9])+)*$`)

var textPBMultiline = prototext.MarshalOptions{
	Multiline: true,
}

// monorailPageSize is the maximum number of issues that can be requested
// through GetIssues at a time. This limit is set by monorail.
const monorailPageSize = 100

// BugManager controls the creation of, and updates to, monorail bugs
// for clusters.
type BugManager struct {
	client *Client
	// The LUCI Project.
	project string
	// The monorail project.
	monorailProject string
	// The generator used to generate updates to monorail bugs.
	// Set if and only if usePolicyBasedManagement.
	generator *RequestGenerator
	// Simulate, if set, tells BugManager not to make mutating changes
	// to monorail but only log the changes it would make. Must be set
	// when running locally as RPCs made from developer systems will
	// appear as that user, which breaks the detection of user-made
	// priority changes vs system-made priority changes.
	Simulate bool
}

// NewBugManager initialises a new bug manager, using the specified
// monorail client.
func NewBugManager(client *Client, uiBaseURL, project string, projectCfg *configpb.ProjectConfig) (*BugManager, error) {
	var g *RequestGenerator
	var monorailProject string
	var err error
	g, err = NewGenerator(uiBaseURL, project, projectCfg)
	if err != nil {
		return nil, errors.Annotate(err, "create issue generator").Err()
	}
	monorailProject = projectCfg.BugManagement.Monorail.Project
	return &BugManager{
		client:          client,
		project:         project,
		monorailProject: monorailProject,
		generator:       g,
		Simulate:        false,
	}, nil
}

// Create creates a new bug for the given request, returning its name, or
// any encountered error.
func (m *BugManager) Create(ctx context.Context, request bugs.BugCreateRequest) bugs.BugCreateResponse {
	var response bugs.BugCreateResponse
	response.Simulated = m.Simulate
	response.PolicyActivationsNotified = make(map[bugs.PolicyID]struct{})

	components := request.MonorailComponents
	components, err := m.filterToValidComponents(ctx, components)
	if err != nil {
		response.Error = errors.Annotate(err, "validate components").Err()
		return response
	}

	makeReq, err := m.generator.PrepareNew(request.RuleID, request.ActivePolicyIDs, request.Description, components)
	if err != nil {
		response.Error = errors.Annotate(err, "prepare new issue").Err()
		return response
	}

	var bugID string
	if m.Simulate {
		logging.Debugf(ctx, "Would create Monorail issue: %s", textPBMultiline.Format(makeReq))
		bugID = fmt.Sprintf("%s/12345678", m.monorailProject)
	} else {
		// Save the issue in Monorail.
		issue, err := m.client.MakeIssue(ctx, makeReq)
		if err != nil {
			response.Error = errors.Annotate(err, "create issue in monorail").Err()
			return response
		}
		bugID, err = fromMonorailIssueName(issue.Name)
		if err != nil {
			response.Error = errors.Annotate(err, "parsing monorail issue name").Err()
			return response
		}
		bugs.BugsCreatedCounter.Add(ctx, 1, m.project, "monorail")
	}
	// A bug was filed.
	response.ID = bugID

	response.PolicyActivationsNotified, err = m.notifyPolicyActivation(ctx, request.RuleID, bugID, request.ActivePolicyIDs)
	if err != nil {
		response.Error = errors.Annotate(err, "notify policy activations").Err()
		return response
	}

	return response
}

// filterToValidComponents limits the given list of components to only those
// components which exist in monorail, and are active.
func (m *BugManager) filterToValidComponents(ctx context.Context, components []string) ([]string, error) {
	var result []string
	for _, c := range components {
		if !componentRE.MatchString(c) {
			continue
		}
		existsAndActive, err := m.client.GetComponentExistsAndActive(ctx, m.monorailProject, c)
		if err != nil {
			return nil, err
		}
		if !existsAndActive {
			continue
		}
		result = append(result, c)
	}
	return result, nil
}

// notifyPolicyActivation notifies that the given policies have activated.
//
// This method supports partial success; it returns the set of policies
// which were successfully notified even if an error is encountered and
// returned.
func (m *BugManager) notifyPolicyActivation(ctx context.Context, ruleID, bugID string, policyIDsToNotify map[bugs.PolicyID]struct{}) (map[bugs.PolicyID]struct{}, error) {
	policiesNotified := make(map[bugs.PolicyID]struct{})

	// Notify policies which have activated in descending priority order.
	sortedPolicyIDToNotify := m.generator.SortPolicyIDsByPriorityDescending(policyIDsToNotify)
	for _, policyID := range sortedPolicyIDToNotify {
		commentRequest, err := m.generator.PreparePolicyActivatedComment(ruleID, bugID, policyID)
		if err != nil {
			return policiesNotified, errors.Annotate(err, "prepare policy activated comment for policy %q", policyID).Err()
		}
		// Only post a comment if the policy has specified one.
		if commentRequest != nil {
			if err := m.applyModification(ctx, commentRequest); err != nil {
				return policiesNotified, errors.Annotate(err, "post policy activated comment for policy %q", policyID).Err()
			}
		}
		// Policy activation successfully notified.
		policiesNotified[policyID] = struct{}{}
	}
	return policiesNotified, nil
}

// Update updates the specified list of bugs.
func (m *BugManager) Update(ctx context.Context, request []bugs.BugUpdateRequest) ([]bugs.BugUpdateResponse, error) {
	// Fetch issues for bugs to update.
	issues, err := m.fetchIssues(ctx, request)
	if err != nil {
		return nil, err
	}

	var responses []bugs.BugUpdateResponse
	for i, req := range request {
		issue := issues[i]
		if issue == nil {
			// The bug does not exist, or is in a different monorail project
			// to the monorail project configured for this project. Take
			// no action.
			responses = append(responses, bugs.BugUpdateResponse{
				IsDuplicate:               false,
				IsDuplicateAndAssigned:    false,
				ShouldArchive:             false,
				PolicyActivationsNotified: map[bugs.PolicyID]struct{}{},
			})
			logging.Warningf(ctx, "Monorail issue %s not found, skipping.", req.Bug.ID)
			continue
		}

		response := m.updateIssue(ctx, req, issue)
		responses = append(responses, response)
	}
	return responses, nil
}

func (m *BugManager) updateIssue(ctx context.Context, request bugs.BugUpdateRequest, issue *mpb.Issue) bugs.BugUpdateResponse {
	var response bugs.BugUpdateResponse
	response.PolicyActivationsNotified = map[bugs.PolicyID]struct{}{}

	// If the context times out part way through an update, we do
	// not know if our bug update succeeded (but we have not received the
	// success response back from monorail yet) or the bug update failed.
	//
	// This is problematic for bug updates that require changes to the
	// bug in tandem with updates to the rule, as we do not know if we
	// need to make the rule update. For example:
	// - Disabling IsManagingBugPriority in tandem with a comment on
	//   the bug indicating the user has taken priority control of the
	//   bug.
	// - Notifying the bug is associated with a rule in tandem with
	//   an update to the bug management state recording we send this
	//   notification.
	//
	// If we incorrectly assume a bug comment was made when it was not,
	// we may fail to deliver comments on bugs.
	// If we incorrectly assume a bug comment was not delivered when it was,
	// we may end up repeatedly making the same comment.
	//
	// We prefer the second over the first, but we try here to reduce the
	// likelihood of either happening by ensuring we have at least one minute
	// of time available.
	if err := bugs.EnsureTimeToDeadline(ctx, time.Minute); err != nil {
		response.Error = err
		return response
	}

	if issue.Status.Status == DuplicateStatus {
		response.IsDuplicate = true
		response.IsDuplicateAndAssigned = issue.Owner.GetUser() != ""
	}
	response.ShouldArchive = shouldArchiveRule(issue, clock.Now(ctx), request.IsManagingBug)
	response.DisableRulePriorityUpdates = false // Set below if necessary.

	if !response.IsDuplicate && !response.ShouldArchive {
		if !request.BugManagementState.RuleAssociationNotified {
			updateRequest, err := m.generator.PrepareRuleAssociatedComment(request.RuleID, request.Bug.ID)
			if err != nil {
				response.Error = errors.Annotate(err, "prepare rule associated comment").Err()
				return response
			}
			if err := m.applyModification(ctx, updateRequest); err != nil {
				response.Error = errors.Annotate(err, "create rule associated comment").Err()
				return response
			}
			response.RuleAssociationNotified = true
		}

		// Identify which policies have activated for the first time and notify them (if any).
		policyIDsToNotify := bugs.ActivePoliciesPendingNotification(request.BugManagementState)

		var err error
		response.PolicyActivationsNotified, err = m.notifyPolicyActivation(ctx, request.RuleID, request.Bug.ID, policyIDsToNotify)
		if err != nil {
			response.Error = errors.Annotate(err, "notify policy activations").Err()
			return response
		}

		// Apply priority and verified updates, as necessary. This should occur
		// after we have notified about policy activation, as that is the more
		// logical order for someone reading the bug.
		needsUpdate, err := m.generator.NeedsPriorityOrVerifiedUpdate(request.BugManagementState, issue, request.IsManagingBugPriority)
		if err != nil {
			response.Error = errors.Annotate(err, "determine if priority/verified update required").Err()
			return response
		}
		if request.IsManagingBug && needsUpdate {
			comments, err := m.client.ListComments(ctx, issue.Name)
			if err != nil {
				response.Error = errors.Annotate(err, "list comments").Err()
				return response
			}
			hasManuallySetPriority := hasManuallySetPriority(comments, request.IsManagingBugPriorityLastUpdated)

			mur, err := m.generator.MakePriorityOrVerifiedUpdate(MakeUpdateOptions{
				RuleID:                 request.RuleID,
				BugManagementState:     request.BugManagementState,
				Issue:                  issue,
				IsManagingBugPriority:  request.IsManagingBugPriority,
				HasManuallySetPriority: hasManuallySetPriority,
			})
			if err != nil {
				response.Error = errors.Annotate(err, "prepare priority/verified update").Err()
				return response
			}
			response.DisableRulePriorityUpdates = mur.disableBugPriorityUpdates
			if err := m.applyModification(ctx, mur.request); err != nil {
				response.Error = errors.Annotate(err, "update monorail issue").Err()
				return response
			}
		}
	}
	return response
}

func (m *BugManager) applyModification(ctx context.Context, modifyRequest *mpb.ModifyIssuesRequest) error {
	if m.Simulate {
		logging.Debugf(ctx, "Would update Monorail issue: %s", textPBMultiline.Format(modifyRequest))
	} else {
		if err := m.client.ModifyIssues(ctx, modifyRequest); err != nil {
			return errors.Annotate(err, "apply modificaton").Err()
		}
		bugs.BugsUpdatedCounter.Add(ctx, 1, m.project, "monorail")
	}
	return nil
}

// shouldArchiveRule determines if the rule managing the given issue should
// be archived.
func shouldArchiveRule(issue *mpb.Issue, now time.Time, isManaging bool) bool {
	// If the bug is set to a status like "Archived", immediately archive
	// the rule as well. We should not re-open such a bug.
	if _, ok := ArchivedStatuses[issue.Status.Status]; ok {
		return true
	}
	if isManaging {
		// If LUCI Analysis is managing the bug,
		// more than 30 days since the issue was verified.
		return issue.Status.Status == VerifiedStatus &&
			now.Sub(issue.StatusModifyTime.AsTime()).Hours() >= 30*24
	} else {
		// If the user is managing the bug,
		// more than 30 days since the issue was closed.
		_, ok := ClosedStatuses[issue.Status.Status]
		return ok &&
			now.Sub(issue.StatusModifyTime.AsTime()).Hours() >= 30*24
	}
}

// GetMergedInto reads the bug (if any) the given bug was merged into.
// If the given bug is not merged into another bug, this returns nil.
func (m *BugManager) GetMergedInto(ctx context.Context, bug bugs.BugID) (*bugs.BugID, error) {
	if bug.System != bugs.MonorailSystem {
		// Indicates an implementation error with the caller.
		panic("monorail bug manager can only deal with monorail bugs")
	}
	name, err := toMonorailIssueName(bug.ID)
	if err != nil {
		return nil, err
	}
	issue, err := m.client.GetIssue(ctx, name)
	if err != nil {
		return nil, err
	}
	result, err := mergedIntoBug(issue)
	if err != nil {
		return nil, errors.Annotate(err, "resolving canoncial merged into bug").Err()
	}
	return result, nil
}

// Unduplicate updates the given bug to no longer be marked as duplicating
// another bug, posting the given message on the bug.
func (m *BugManager) UpdateDuplicateSource(ctx context.Context, request bugs.UpdateDuplicateSourceRequest) error {
	if request.BugDetails.Bug.System != bugs.MonorailSystem {
		// Indicates an implementation error with the caller.
		panic("monorail bug manager can only deal with monorail bugs")
	}
	req, err := m.generator.UpdateDuplicateSource(request.BugDetails.Bug.ID, request.ErrorMessage, request.BugDetails.RuleID, request.DestinationRuleID)
	if err != nil {
		return errors.Annotate(err, "mark issue as available").Err()
	}
	if m.Simulate {
		logging.Debugf(ctx, "Would update Monorail issue: %s", textPBMultiline.Format(req))
	} else {
		if err := m.client.ModifyIssues(ctx, req); err != nil {
			return errors.Annotate(err, "failed to update duplicate source monorail issue %s", request.BugDetails.Bug.ID).Err()
		}
	}
	return nil
}

var buganizerExtRefRe = regexp.MustCompile(`^b/([1-9][0-9]{0,16})$`)

// mergedIntoBug determines if the given bug is a duplicate of another
// bug, and if so, what the identity of that bug is.
func mergedIntoBug(issue *mpb.Issue) (*bugs.BugID, error) {
	if issue.Status.Status == DuplicateStatus &&
		issue.MergedIntoIssueRef != nil {
		if issue.MergedIntoIssueRef.Issue != "" {
			name, err := fromMonorailIssueName(issue.MergedIntoIssueRef.Issue)
			if err != nil {
				// This should not happen unless monorail or the
				// implementation here is broken.
				return nil, err
			}
			return &bugs.BugID{
				System: bugs.MonorailSystem,
				ID:     name,
			}, nil
		}
		matches := buganizerExtRefRe.FindStringSubmatch(issue.MergedIntoIssueRef.ExtIdentifier)
		if matches == nil {
			// A non-buganizer external issue tracker was used. This is not
			// supported by us, treat the issue as not duplicate of something
			// else and let auto-updating kick the bug out of duplicate state
			// if there is still impact. The user should manually resolve the
			// situation.
			return nil, fmt.Errorf("unsupported non-monorail non-buganizer bug reference: %s", issue.MergedIntoIssueRef.ExtIdentifier)
		}
		return &bugs.BugID{
			System: bugs.BuganizerSystem,
			ID:     matches[1],
		}, nil
	}
	return nil, nil
}

// fetchIssues fetches monorail issues using the internal bug names like
// {monorail_project}/{issue_id}. Issues in the result will be in 1:1
// correspondence (by index) to the request. If an issue does not exist,
// or is from a monorail project other than the one configured for this
// LUCI project, the corresponding item in the response will be nil.
func (m *BugManager) fetchIssues(ctx context.Context, request []bugs.BugUpdateRequest) ([]*mpb.Issue, error) {
	// Calculate the number of requests required, rounding up
	// to the nearest page.
	pages := (len(request) + (monorailPageSize - 1)) / monorailPageSize

	response := make([]*mpb.Issue, 0, len(request))
	for i := 0; i < pages; i++ {
		// Divide names into pages of monorailPageSize.
		pageEnd := (i + 1) * monorailPageSize
		if pageEnd > len(request) {
			pageEnd = len(request)
		}
		requestPage := request[i*monorailPageSize : pageEnd]

		var ids []string
		for _, requestItem := range requestPage {
			if requestItem.Bug.System != bugs.MonorailSystem {
				// Indicates an implementation error with the caller.
				panic("monorail bug manager can only deal with monorail bugs")
			}
			monorailProject, id, err := toMonorailProjectAndID(requestItem.Bug.ID)
			if err != nil {
				return nil, err
			}
			if monorailProject != m.monorailProject {
				// Only query bugs from the same monorail project as what has
				// been configured for the LUCI Project.
				continue
			}
			ids = append(ids, id)
		}

		// Guarantees result array in 1:1 correspondence to requested IDs.
		issues, err := m.client.BatchGetIssues(ctx, m.monorailProject, ids)
		if err != nil {
			return nil, err
		}
		response = append(response, issues...)
	}
	return response, nil
}

// toMonorailProjectAndID splits an internal bug name like
// "{monorail_project}/{numeric_id}" to the monorail project and
// numeric ID.
func toMonorailProjectAndID(bug string) (project, id string, err error) {
	parts := bugs.MonorailBugIDRe.FindStringSubmatch(bug)
	if parts == nil {
		return "", "", fmt.Errorf("invalid bug %q", bug)
	}
	return parts[1], parts[2], nil
}

// toMonorailIssueName converts an internal bug name like
// "{monorail_project}/{numeric_id}" to a monorail issue name like
// "projects/{project}/issues/{numeric_id}".
func toMonorailIssueName(bug string) (string, error) {
	parts := bugs.MonorailBugIDRe.FindStringSubmatch(bug)
	if parts == nil {
		return "", fmt.Errorf("invalid bug %q", bug)
	}
	return fmt.Sprintf("projects/%s/issues/%s", parts[1], parts[2]), nil
}

// fromMonorailIssueName converts a monorail issue name like
// "projects/{project}/issues/{numeric_id}" to an internal bug name like
// "{monorail_project}/{numeric_id}".
func fromMonorailIssueName(name string) (string, error) {
	parts := monorailRe.FindStringSubmatch(name)
	if parts == nil {
		return "", fmt.Errorf("invalid monorail issue name %q", name)
	}
	return fmt.Sprintf("%s/%s", parts[1], parts[2]), nil
}
