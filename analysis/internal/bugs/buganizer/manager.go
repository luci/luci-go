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

// Package buganizer contains methods to manage issues in Issue Tracker
// linked to failure association rules.
package buganizer

import (
	"context"
	"strconv"
	"time"

	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/third_party/google.golang.org/genproto/googleapis/devtools/issuetracker/v1"

	"go.chromium.org/luci/analysis/internal/bugs"
	configpb "go.chromium.org/luci/analysis/proto/config"
)

// The maximum number of issues you can get from Buganizer
// in one BatchGetIssues RPC.
// This is set by Buganizer.
const maxPageSize = 100

var textPBMultiline = prototext.MarshalOptions{
	Multiline: true,
}

// Client represents the interface needed by the bug manager
// to manipulate issues in Google Issue Tracker.
type Client interface {
	// Closes the underlying client.
	Close()
	// BatchGetIssues returns a list of issues matching the BatchGetIssuesRequest.
	BatchGetIssues(ctx context.Context, in *issuetracker.BatchGetIssuesRequest) (*issuetracker.BatchGetIssuesResponse, error)
	// GetComponent gets a component, including its parent hierarchy info.
	GetComponent(ctx context.Context, in *issuetracker.GetComponentRequest) (*issuetracker.Component, error)
	// GetIssue returns data about a single issue.
	GetIssue(ctx context.Context, in *issuetracker.GetIssueRequest) (*issuetracker.Issue, error)
	// CreateIssue creates an issue using the data provided.
	CreateIssue(ctx context.Context, in *issuetracker.CreateIssueRequest) (*issuetracker.Issue, error)
	// ModifyIssue modifies an issue using the data provided.
	ModifyIssue(ctx context.Context, in *issuetracker.ModifyIssueRequest) (*issuetracker.Issue, error)
	// ListIssueUpdates lists the updates which occured in an issue, it returns a delegate to an IssueUpdateIterator.
	// The iterator can be used to fetch IssueUpdates one by one.
	ListIssueUpdates(ctx context.Context, in *issuetracker.ListIssueUpdatesRequest) IssueUpdateIterator
	// CreateIssueComment creates an issue comment using the data provided.
	CreateIssueComment(ctx context.Context, in *issuetracker.CreateIssueCommentRequest) (*issuetracker.IssueComment, error)
	// UpdateIssueComment updates an issue comment and returns the updated comment.
	UpdateIssueComment(ctx context.Context, in *issuetracker.UpdateIssueCommentRequest) (*issuetracker.IssueComment, error)
	// ListIssueComments lists issue comments, it returns a delegate to an IssueCommentIterator.
	// The iterator can be used to fetch IssueComment one by one.
	ListIssueComments(ctx context.Context, in *issuetracker.ListIssueCommentsRequest) IssueCommentIterator
	// GetAutomationAccess checks that automation has permission on a resource.
	// Does not require any permission on the resource
	GetAutomationAccess(ctx context.Context, in *issuetracker.GetAutomationAccessRequest) (*issuetracker.GetAutomationAccessResponse, error)
	// CreateHotlistEntry adds an issue to a hotlist.
	CreateHotlistEntry(ctx context.Context, in *issuetracker.CreateHotlistEntryRequest) (*issuetracker.HotlistEntry, error)
}

// An interface for an IssueUpdateIterator.
type IssueUpdateIterator interface {
	// Next returns the next update in the list of updates.
	// If the error is iterator.Done, this means that the iterator is exhausted.
	// Once iterator.Done is returned, it will always be returned thereafter.
	Next() (*issuetracker.IssueUpdate, error)
}

// An interface for the IssueCommentIterator.
type IssueCommentIterator interface {
	// Next returns the next comment in the list of comments.
	// If the error is iterator.Done, this means that the iterator is exhausted.
	// Once iterator.Done is returned, it will always be returned thereafter.
	Next() (*issuetracker.IssueComment, error)
}

type BugManager struct {
	client Client
	// The email address of the LUCI Analysis instance. This is used to distinguish
	// priority updates made by LUCI Analysis itself from those made by others.
	selfEmail string
	// The LUCI Project.
	project string
	// The default buganizer component to file into.
	defaultComponent *configpb.BuganizerComponent
	// The generator used to generate updates to Buganizer bugs.
	// Set if and only if usePolicyBasedManagement.
	requestGenerator *RequestGenerator
}

// NewBugManager creates a new Buganizer bug manager than can be
// used to manipulate bugs in Buganizer.
func NewBugManager(client Client,
	uiBaseURL, project, selfEmail string,
	projectCfg *configpb.ProjectConfig) (*BugManager, error) {

	generator, err := NewRequestGenerator(
		client,
		project,
		uiBaseURL,
		selfEmail,
		projectCfg,
	)
	if err != nil {
		return nil, errors.Fmt("create request generator: %w", err)
	}
	defaultComponent := projectCfg.BugManagement.Buganizer.DefaultComponent

	return &BugManager{
		client:           client,
		defaultComponent: defaultComponent,
		project:          project,
		selfEmail:        selfEmail,
		requestGenerator: generator,
	}, nil
}

// Create creates an issue in Buganizer and returns the issue ID.
func (bm *BugManager) Create(ctx context.Context, createRequest bugs.BugCreateRequest) bugs.BugCreateResponse {
	var response bugs.BugCreateResponse
	response.Simulated = false // Simulation not supported for Buganizer.
	response.PolicyActivationsNotified = make(map[bugs.PolicyID]struct{})

	componentID := bm.defaultComponent.Id
	buganizerTestMode := ctx.Value(&BuganizerTestModeKey)
	wantedComponentID := createRequest.BuganizerComponent

	fallbackReason := FallbackReason_None

	// Use wanted component if not in test mode.
	if buganizerTestMode == nil || !buganizerTestMode.(bool) {
		if wantedComponentID != componentID && wantedComponentID > 0 {
			componentChecker := NewComponentAccessChecker(bm.client, bm.selfEmail)

			access, err := componentChecker.CheckAccess(ctx, wantedComponentID)
			if err != nil {
				response.Error = errors.Fmt("check access to create Buganizer issue: %w", err)
				return response
			}
			if !access.Appender || !access.IssueDefaultsAppender {
				fallbackReason = FallbackReason_NoPermission
			} else if access.IsArchived {
				fallbackReason = FallbackReason_ComponentArchived
			} else {
				componentID = createRequest.BuganizerComponent
			}
		}
	} else {
		// In the staging environment, the wanted components do not exist. Treat
		// this as a no permission error.
		fallbackReason = FallbackReason_NoPermission
	}

	component, err := bm.client.GetComponent(ctx, &issuetracker.GetComponentRequest{ComponentId: componentID})
	if err != nil {
		response.Error = errors.Fmt("get Buganizer component: %w", err)
		return response
	}

	createIssueRequest, err := bm.requestGenerator.PrepareNew(
		createRequest.Description,
		createRequest.ActivePolicyIDs,
		createRequest.RuleID,
		component,
	)
	if err != nil {
		response.Error = errors.Fmt("prepare new issue: %w", err)
		return response
	}

	var issueID int64
	issue, err := bm.client.CreateIssue(ctx, createIssueRequest)
	if err != nil {
		response.Error = errors.Fmt("create Buganizer issue: %w", err)
		return response
	}
	issueID = issue.IssueId
	bugs.BugsCreatedCounter.Add(ctx, 1, bm.project, "buganizer")

	// A bug was filed.
	response.ID = strconv.Itoa(int(issueID))

	if fallbackReason != FallbackReason_None {
		commentRequest := bm.requestGenerator.PrepareComponentFallbackComment(issueID, wantedComponentID, fallbackReason)
		if _, err := bm.client.CreateIssueComment(ctx, commentRequest); err != nil {
			response.Error = errors.Fmt("create issue link comment: %w", err)
			return response
		}
	}

	response.PolicyActivationsNotified, err = bm.notifyPolicyActivation(ctx, createRequest.RuleID, issueID, createRequest.ActivePolicyIDs)
	if err != nil {
		response.Error = errors.Fmt("notify policy activations: %w", err)
		return response
	}

	hotlistIDs := bm.requestGenerator.ExpectedHotlistIDs(createRequest.ActivePolicyIDs)
	if err := bm.insertIntoHotlists(ctx, hotlistIDs, issueID); err != nil {
		response.Error = errors.Fmt("insert into hotlists: %w", err)
		return response
	}

	return response
}

// notifyPolicyActivation notifies that the given policies have activated.
//
// This method supports partial success; it returns the set of policies
// which were successfully notified even if an error is encountered and
// returned.
func (bm *BugManager) notifyPolicyActivation(ctx context.Context, ruleID string, issueID int64, policyIDsToNotify map[bugs.PolicyID]struct{}) (map[bugs.PolicyID]struct{}, error) {
	policiesNotified := make(map[bugs.PolicyID]struct{})

	// Notify policies which have activated in descending priority order.
	sortedPolicyIDToNotify := bm.requestGenerator.SortPolicyIDsByPriorityDescending(policyIDsToNotify)
	for _, policyID := range sortedPolicyIDToNotify {
		commentRequest, err := bm.requestGenerator.PreparePolicyActivatedComment(ruleID, issueID, policyID)
		if err != nil {
			return policiesNotified, errors.Fmt("prepare comment for policy %q: %w", policyID, err)
		}
		// Only post a comment if the policy has specified one.
		if commentRequest != nil {
			if err := bm.createIssueComment(ctx, commentRequest); err != nil {
				return policiesNotified, errors.Fmt("post comment for policy %q: %w", policyID, err)
			}
		}
		// Policy activation successfully notified.
		policiesNotified[policyID] = struct{}{}
	}
	return policiesNotified, nil
}

// maintainHotlists ensures the has been inserted into the hotlists
// configured by the active policies. Note: The issue is not removed
// from the hotlist when a policy de-activates on a rule.
func (bm *BugManager) insertIntoHotlists(ctx context.Context, hotlistIDs map[int64]struct{}, issueID int64) error {
	hotlistInsertionRequests := PrepareHotlistInsertions(hotlistIDs, issueID)
	for _, req := range hotlistInsertionRequests {
		if _, err := bm.client.CreateHotlistEntry(ctx, req); err != nil {
			return errors.Fmt("insert into hotlist %d: %w", req.HotlistId, err)
		}
	}
	return nil
}

// Update updates the issues in Buganizer.
func (bm *BugManager) Update(ctx context.Context, requests []bugs.BugUpdateRequest) ([]bugs.BugUpdateResponse, error) {
	issues, err := bm.fetchIssues(ctx, requests)
	if err != nil {
		return nil, errors.Fmt("fetch issues for update: %w", err)
	}

	issuesByID := make(map[int64]*issuetracker.Issue)
	for _, fetchedIssue := range issues {
		issuesByID[fetchedIssue.IssueId] = fetchedIssue
	}

	var responses []bugs.BugUpdateResponse
	for _, request := range requests {
		id, err := strconv.ParseInt(request.Bug.ID, 10, 64)
		if err != nil {
			// This should never occur here, as we do a similar conversion in fetchIssues.
			return nil, errors.Fmt("convert bug id to int: %w", err)
		}
		issue, ok := issuesByID[id]
		if !ok {
			// The bug does not exist, or is in a different buganizer project
			// to the buganizer project configured for this project
			// or we have no permission to access it.
			// Take no action.
			responses = append(responses, bugs.BugUpdateResponse{
				IsDuplicate:               false,
				IsDuplicateAndAssigned:    false,
				ShouldArchive:             false,
				PolicyActivationsNotified: make(map[bugs.PolicyID]struct{}),
			})
			logging.Fields{
				"Project":        bm.project,
				"BuganizerBugID": request.Bug.ID,
			}.Warningf(ctx, "Buganizer issue %s not found or we don't have permission to access it (project: %s), skipping.", request.Bug.ID, bm.project)
			continue
		}
		updateResponse := bm.updateIssue(ctx, request, issue)
		responses = append(responses, updateResponse)

		fields := logging.Fields{
			"Project":                    bm.project,
			"BuganizerBugID":             request.Bug.ID,
			"ShouldArchive":              updateResponse.ShouldArchive,
			"DisableRulePriorityUpdates": updateResponse.DisableRulePriorityUpdates,
			"IsDuplicate":                updateResponse.IsDuplicate,
			"IsDuplicateAndAssigned":     updateResponse.IsDuplicateAndAssigned,
		}
		if updateResponse.Error != nil {
			fields.Errorf(ctx, "Error updating buganizer issue (%s): %s.", request.Bug.ID, updateResponse.Error)
		} else {
			fields.Debugf(ctx, "Ran updates for buganizer issue (%s), see fields for details.", request.Bug.ID)
		}
	}
	return responses, nil
}

// updateIssue updates the given issue, adjusting its priority,
// and verify or unverifying it.
func (bm *BugManager) updateIssue(ctx context.Context, request bugs.BugUpdateRequest, issue *issuetracker.Issue) bugs.BugUpdateResponse {
	var response bugs.BugUpdateResponse
	response.PolicyActivationsNotified = map[bugs.PolicyID]struct{}{}

	// If the context times out part way through an update, we do
	// not know if our bug update succeeded (but we have not received the
	// success response back from Buganizer yet) or the bug update failed.
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

	response.ShouldArchive = shouldArchiveRule(ctx, issue, request.IsManagingBug)
	if issue.IssueState.Status == issuetracker.Issue_DUPLICATE {
		response.IsDuplicate = true
		response.IsDuplicateAndAssigned = issue.IssueState.Assignee != nil
	}

	if !response.IsDuplicate && !response.ShouldArchive {
		if !request.BugManagementState.RuleAssociationNotified {
			commentRequest, err := bm.requestGenerator.PrepareRuleAssociatedComment(request.RuleID, issue.IssueId)
			if err != nil {
				response.Error = errors.Fmt("prepare rule associated comment: %w", err)
				return response
			}
			if err := bm.createIssueComment(ctx, commentRequest); err != nil {
				response.Error = errors.Fmt("create rule associated comment: %w", err)
				return response
			}
			response.RuleAssociationNotified = true
		}

		// Identify which policies have activated for the first time and notify them.
		policyIDsToNotify := bugs.ActivePoliciesPendingNotification(request.BugManagementState)

		var err error
		response.PolicyActivationsNotified, err = bm.notifyPolicyActivation(ctx, request.RuleID, issue.IssueId, policyIDsToNotify)
		if err != nil {
			response.Error = errors.Fmt("notify policy activations: %w", err)
			return response
		}

		// Apply priority/verified updates.
		if request.IsManagingBug && bm.requestGenerator.NeedsPriorityOrVerifiedUpdate(request.BugManagementState, issue, request.IsManagingBugPriority) {
			// List issue updates.
			listUpdatesRequest := &issuetracker.ListIssueUpdatesRequest{
				IssueId: issue.IssueId,
			}
			it := bm.client.ListIssueUpdates(ctx, listUpdatesRequest)

			// Determine if bug priority manually set. This involves listing issue comments.
			hasManuallySetPriority, err := bm.hasManuallySetPriority(it, bm.selfEmail, request.IsManagingBugPriorityLastUpdated)
			if err != nil {
				response.Error = errors.Fmt("determine if priority manually set: %w", err)
				return response
			}
			mur, err := bm.requestGenerator.MakePriorityOrVerifiedUpdate(MakeUpdateOptions{
				RuleID:                 request.RuleID,
				BugManagementState:     request.BugManagementState,
				Issue:                  issue,
				IsManagingBugPriority:  request.IsManagingBugPriority,
				HasManuallySetPriority: hasManuallySetPriority,
			})
			if err != nil {
				response.Error = errors.Fmt("create update request for issue: %w", err)
				return response
			}
			if _, err := bm.client.ModifyIssue(ctx, mur.request); err != nil {
				response.Error = errors.Fmt("update Buganizer issue: %w", err)
				return response
			}
			bugs.BugsUpdatedCounter.Add(ctx, 1, bm.project, "buganizer")
			response.DisableRulePriorityUpdates = mur.disablePriorityUpdates
		}

		// Hotlists
		// Find all hotlists specified on active policies.
		hotlistsIDsToAdd := bm.requestGenerator.ExpectedHotlistIDs(bugs.ActivePolicies(request.BugManagementState))

		// Subtract the hotlists already on the bug.
		for _, hotlistID := range issue.IssueState.HotlistIds {
			delete(hotlistsIDsToAdd, hotlistID)
		}

		if err := bm.insertIntoHotlists(ctx, hotlistsIDsToAdd, issue.IssueId); err != nil {
			response.Error = errors.Fmt("insert issue into hotlists: %w", err)
			return response
		}
	}

	return response
}

func (bm *BugManager) createIssueComment(ctx context.Context, commentRequest *issuetracker.CreateIssueCommentRequest) error {
	// Only post a comment if the policy has specified one.
	if _, err := bm.client.CreateIssueComment(ctx, commentRequest); err != nil {
		return errors.Fmt("create comment: %w", err)
	}
	bugs.BugsUpdatedCounter.Add(ctx, 1, bm.project, "buganizer")
	return nil
}

// hasManuallySetPriority checks whether this issue's priority was last modified by
// a user.
func (bm *BugManager) hasManuallySetPriority(
	it IssueUpdateIterator, selfEmail string, isManagingBugPriorityLastUpdated time.Time) (bool, error) {
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
			return false, errors.Fmt("iterating through issue updates: %w", err)
		}
		if update.Author.EmailAddress != selfEmail {
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
		priorityUpdateTime.After(isManagingBugPriorityLastUpdated) {
		return true, nil
	}
	return false, nil
}

func shouldArchiveRule(ctx context.Context, issue *issuetracker.Issue, isManaging bool) bool {
	// If the bug is set to a status like "Archived", immediately archive
	// the rule as well. We should not re-open such a bug.
	if issue.IsArchived {
		return true
	}
	now := clock.Now(ctx)
	if isManaging {
		// If LUCI Analysis is managing the bug,
		// more than 30 days since the issue was verified.
		return issue.IssueState.Status == issuetracker.Issue_VERIFIED &&
			issue.VerifiedTime.IsValid() &&
			now.Sub(issue.VerifiedTime.AsTime()).Hours() >= 30*24
	} else {
		// If the user is managing the bug,
		// more than 30 days since the issue was closed.
		_, ok := ClosedStatuses[issue.IssueState.Status]
		return ok && issue.ResolvedTime.IsValid() &&
			now.Sub(issue.ResolvedTime.AsTime()).Hours() >= 30*24
	}
}

func (bm *BugManager) fetchIssues(ctx context.Context, requests []bugs.BugUpdateRequest) ([]*issuetracker.Issue, error) {
	issues := make([]*issuetracker.Issue, 0, len(requests))

	chunks := chunkRequests(requests)

	for _, chunk := range chunks {
		ids := make([]int64, 0, len(chunk))
		for _, request := range chunk {
			if request.Bug.System != bugs.BuganizerSystem {
				// Indicates an implementation error with the caller.
				panic("Buganizer bug manager can only deal with Buganizer bugs")
			}
			id, err := strconv.Atoi(request.Bug.ID)
			if err != nil {
				return nil, errors.Fmt("convert bug id to int: %w", err)
			}
			ids = append(ids, int64(id))
		}

		fetchedIssues, err := bm.client.BatchGetIssues(ctx, &issuetracker.BatchGetIssuesRequest{
			IssueIds: ids,
			View:     issuetracker.IssueView_FULL,
		})
		if err != nil {
			return nil, errors.Fmt("fetch issues: %w", err)
		}
		issues = append(issues, fetchedIssues.Issues...)
	}
	return issues, nil
}

// chunkRequests creates chunks of bug requests that can be used to fetch issues.
func chunkRequests(requests []bugs.BugUpdateRequest) [][]bugs.BugUpdateRequest {
	// Calculate the number of chunks
	numChunks := (len(requests) / maxPageSize) + 1
	chunks := make([][]bugs.BugUpdateRequest, 0, numChunks)
	total := len(requests)

	for i := 0; i < total; i += maxPageSize {
		var end int
		if i+maxPageSize < total {
			end = i + maxPageSize
		} else {
			end = total
		}
		chunks = append(chunks, requests[i:end])
	}

	return chunks
}

// GetMergedInto returns the canonical bug id that this issue is merged into.
func (bm *BugManager) GetMergedInto(ctx context.Context, bug bugs.BugID) (*bugs.BugID, error) {
	if bug.System != bugs.BuganizerSystem {
		// Indicates an implementation error with the caller.
		panic("Buganizer bug manager can only deal with Buganizer bugs")
	}
	issueId, err := strconv.Atoi(bug.ID)
	if err != nil {
		return nil, errors.Fmt("get merged into: %w", err)
	}
	issue, err := bm.client.GetIssue(ctx, &issuetracker.GetIssueRequest{
		IssueId: int64(issueId),
	})
	if err != nil {
		return nil, err
	}
	result, err := mergedIntoBug(issue)
	if err != nil {
		return nil, errors.Fmt("resolving canoncial merged into bug: %w", err)
	}
	return result, nil
}

// mergedIntoBug determines if the given bug is a duplicate of another
// bug, and if so, what the identity of that bug is.
func mergedIntoBug(issue *issuetracker.Issue) (*bugs.BugID, error) {
	if issue.IssueState.Status == issuetracker.Issue_DUPLICATE &&
		issue.IssueState.CanonicalIssueId > 0 {
		return &bugs.BugID{
			System: bugs.BuganizerSystem,
			ID:     strconv.FormatInt(issue.IssueState.CanonicalIssueId, 10),
		}, nil
	}
	return nil, nil
}

// UpdateDuplicateSource updates the source bug of a duplicate
// bug relationship.
// It normally posts a message advising the user LUCI Analysis
// has merged the rule for the source bug to the destination
// (merged-into) bug, and provides a new link to the failure
// association rule.
// If a cycle was detected, it instead posts a message that the
// duplicate bug could not be handled and marks the bug no
// longer a duplicate to break the cycle.
func (bm *BugManager) UpdateDuplicateSource(ctx context.Context, request bugs.UpdateDuplicateSourceRequest) error {
	if request.BugDetails.Bug.System != bugs.BuganizerSystem {
		// Indicates an implementation error with the caller.
		panic("Buganizer bug manager can only deal with Buganizer bugs")
	}
	issueId, err := strconv.Atoi(request.BugDetails.Bug.ID)
	if err != nil {
		return errors.Fmt("update duplicate source: %w", err)
	}
	req := bm.requestGenerator.UpdateDuplicateSource(int64(issueId), request.ErrorMessage, request.BugDetails.RuleID, request.DestinationRuleID, request.BugDetails.IsAssigned)
	if _, err := bm.client.ModifyIssue(ctx, req); err != nil {
		return errors.Fmt("failed to update duplicate source Buganizer issue %s: %w", request.BugDetails.Bug.ID, err)
	}
	return nil
}
