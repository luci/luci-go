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
	"strconv"
	"strings"
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
	// This flags toggles the bug manager to stub the calls to
	// Buganizer and mock the responses and behaviour of issue manipluation.
	// Use this flag for testing purposes ONLY.
	Simulate bool
}

// NewBugManager creates a new Buganizer bug manager than can be
// used to manipulate bugs in Buganizer.
// Use the `simulate` flag to use the manager in simulation mode
// while testing.
func NewBugManager(client Client,
	uiBaseURL, project, selfEmail string,
	projectCfg *configpb.ProjectConfig,
	simulate bool) (*BugManager, error) {

	generator, err := NewRequestGenerator(
		client,
		project,
		uiBaseURL,
		selfEmail,
		projectCfg,
	)
	if err != nil {
		return nil, errors.Annotate(err, "create request generator").Err()
	}
	defaultComponent := projectCfg.BugManagement.Buganizer.DefaultComponent

	return &BugManager{
		client:           client,
		defaultComponent: defaultComponent,
		project:          project,
		selfEmail:        selfEmail,
		requestGenerator: generator,
		Simulate:         simulate,
	}, nil
}

// Create creates an issue in Buganizer and returns the issue ID.
func (bm *BugManager) Create(ctx context.Context, createRequest bugs.BugCreateRequest) bugs.BugCreateResponse {
	var response bugs.BugCreateResponse
	response.Simulated = bm.Simulate
	response.PolicyActivationsNotified = make(map[bugs.PolicyID]struct{})

	componentID := bm.defaultComponent.Id
	buganizerTestMode := ctx.Value(&BuganizerTestModeKey)
	wantedComponentID := createRequest.BuganizerComponent
	// Use wanted component if not in test mode.
	if buganizerTestMode == nil || !buganizerTestMode.(bool) {
		if wantedComponentID != componentID && wantedComponentID > 0 {
			permissions, err := bm.checkComponentPermissions(ctx, wantedComponentID)
			if err != nil {
				response.Error = errors.Annotate(err, "check permissions to create Buganizer issue").Err()
				return response
			}
			if permissions.appender && permissions.issueDefaultsAppender {
				componentID = createRequest.BuganizerComponent
			}
		}
	}

	createIssueRequest, err := bm.requestGenerator.PrepareNew(
		createRequest.Description,
		createRequest.ActivePolicyIDs,
		createRequest.RuleID,
		componentID,
	)
	if err != nil {
		response.Error = errors.Annotate(err, "prepare new issue").Err()
		return response
	}

	var issueID int64
	if bm.Simulate {
		logging.Debugf(ctx, "Would create Buganizer issue: %s", textPBMultiline.Format(createIssueRequest))
		issueID = 123456
	} else {
		issue, err := bm.client.CreateIssue(ctx, createIssueRequest)
		if err != nil {
			response.Error = errors.Annotate(err, "create Buganizer issue").Err()
			return response
		}
		issueID = issue.IssueId
		bugs.BugsCreatedCounter.Add(ctx, 1, bm.project, "buganizer")
	}
	// A bug was filed.
	response.ID = strconv.Itoa(int(issueID))

	if wantedComponentID > 0 && wantedComponentID != componentID {
		commentRequest := bm.requestGenerator.PrepareNoPermissionComment(issueID, wantedComponentID)
		if bm.Simulate {
			logging.Debugf(ctx, "Would post comment on Buganizer issue: %s", textPBMultiline.Format(commentRequest))
		} else {
			if _, err := bm.client.CreateIssueComment(ctx, commentRequest); err != nil {
				response.Error = errors.Annotate(err, "create issue link comment").Err()
				return response
			}
		}
	}

	response.PolicyActivationsNotified, err = bm.notifyPolicyActivation(ctx, createRequest.RuleID, issueID, createRequest.ActivePolicyIDs)
	if err != nil {
		response.Error = errors.Annotate(err, "notify policy activations").Err()
		return response
	}

	hotlistIDs := bm.requestGenerator.ExpectedHotlistIDs(createRequest.ActivePolicyIDs)
	if err := bm.insertIntoHotlists(ctx, hotlistIDs, issueID); err != nil {
		response.Error = errors.Annotate(err, "insert into hotlists").Err()
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
			return policiesNotified, errors.Annotate(err, "prepare comment for policy %q", policyID).Err()
		}
		// Only post a comment if the policy has specified one.
		if commentRequest != nil {
			if err := bm.createIssueComment(ctx, commentRequest); err != nil {
				return policiesNotified, errors.Annotate(err, "post comment for policy %q", policyID).Err()
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
		if bm.Simulate {
			logging.Debugf(ctx, "Would create hotlist entry: %s", textPBMultiline.Format(req))
		} else {
			if _, err := bm.client.CreateHotlistEntry(ctx, req); err != nil {
				return errors.Annotate(err, "insert into hotlist %d", req.HotlistId).Err()
			}
		}
	}
	return nil
}

// Update updates the issues in Buganizer.
func (bm *BugManager) Update(ctx context.Context, requests []bugs.BugUpdateRequest) ([]bugs.BugUpdateResponse, error) {
	issues, err := bm.fetchIssues(ctx, requests)
	if err != nil {
		return nil, errors.Annotate(err, "fetch issues for update").Err()
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
			return nil, errors.Annotate(err, "convert bug id to int").Err()
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
			logging.Warningf(ctx, "Buganizer issue %s not found or we don't have permission to access it, skipping.", request.Bug.ID)
			continue
		}
		updateResponse := bm.updateIssue(ctx, request, issue)
		responses = append(responses, updateResponse)
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
				response.Error = errors.Annotate(err, "prepare rule associated comment").Err()
				return response
			}
			if err := bm.createIssueComment(ctx, commentRequest); err != nil {
				response.Error = errors.Annotate(err, "create rule associated comment").Err()
				return response
			}
			response.RuleAssociationNotified = true
		}

		// Identify which policies have activated for the first time and notify them.
		policyIDsToNotify := bugs.ActivePoliciesPendingNotification(request.BugManagementState)

		var err error
		response.PolicyActivationsNotified, err = bm.notifyPolicyActivation(ctx, request.RuleID, issue.IssueId, policyIDsToNotify)
		if err != nil {
			response.Error = errors.Annotate(err, "notify policy activations").Err()
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
				response.Error = errors.Annotate(err, "determine if priority manually set").Err()
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
				response.Error = errors.Annotate(err, "create update request for issue").Err()
				return response
			}
			if bm.Simulate {
				logging.Debugf(ctx, "Would update Buganizer issue: %s", textPBMultiline.Format(mur.request))
			} else {
				if _, err := bm.client.ModifyIssue(ctx, mur.request); err != nil {
					response.Error = errors.Annotate(err, "update Buganizer issue").Err()
					return response
				}
				bugs.BugsUpdatedCounter.Add(ctx, 1, bm.project, "buganizer")
			}
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
			response.Error = errors.Annotate(err, "insert issue into hotlists").Err()
			return response
		}
	}

	return response
}

func (bm *BugManager) createIssueComment(ctx context.Context, commentRequest *issuetracker.CreateIssueCommentRequest) error {
	// Only post a comment if the policy has specified one.
	if bm.Simulate {
		logging.Debugf(ctx, "Would post comment on Buganizer issue: %s", textPBMultiline.Format(commentRequest))
	} else {
		if _, err := bm.client.CreateIssueComment(ctx, commentRequest); err != nil {
			return errors.Annotate(err, "create comment").Err()
		}
		bugs.BugsUpdatedCounter.Add(ctx, 1, bm.project, "buganizer")
	}
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
			return false, errors.Annotate(err, "iterating through issue updates").Err()
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
		hourDiff := now.Sub(issue.ModifiedTime.AsTime()).Hours()
		return issue.IssueState.Status == issuetracker.Issue_VERIFIED &&
			hourDiff >= 30*24
	} else {
		// If the user is managing the bug,
		// more than 30 days since the issue was closed.
		_, ok := ClosedStatuses[issue.IssueState.Status]
		return ok &&
			now.Sub(issue.ModifiedTime.AsTime()).Hours() >= 30*24
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
				return nil, errors.Annotate(err, "convert bug id to int").Err()
			}
			ids = append(ids, int64(id))
		}

		fetchedIssues, err := bm.client.BatchGetIssues(ctx, &issuetracker.BatchGetIssuesRequest{
			IssueIds: ids,
			View:     issuetracker.IssueView_FULL,
		})
		if err != nil {
			return nil, errors.Annotate(err, "fetch issues").Err()
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
		return nil, errors.Annotate(err, "get merged into").Err()
	}
	issue, err := bm.client.GetIssue(ctx, &issuetracker.GetIssueRequest{
		IssueId: int64(issueId),
	})
	if err != nil {
		return nil, err
	}
	result, err := mergedIntoBug(issue)
	if err != nil {
		return nil, errors.Annotate(err, "resolving canoncial merged into bug").Err()
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
		return errors.Annotate(err, "update duplicate source").Err()
	}
	req := bm.requestGenerator.UpdateDuplicateSource(int64(issueId), request.ErrorMessage, request.BugDetails.RuleID, request.DestinationRuleID, request.BugDetails.IsAssigned)
	if bm.Simulate {
		logging.Debugf(ctx, "Would update Buganizer issue: %s", textPBMultiline.Format(req))
	} else {
		if _, err := bm.client.ModifyIssue(ctx, req); err != nil {
			return errors.Annotate(err, "failed to update duplicate source Buganizer issue %s", request.BugDetails.Bug.ID).Err()
		}
	}
	return nil
}

// componentPermissions contains the results of checking the permissions of a
// Buganizer component.
type componentPermissions struct {
	// appender is permission to create issues in this component.
	appender bool
	// issueDefaultsAppender is permission to add comments to issues in
	// this component.
	issueDefaultsAppender bool
}

// checkComponentPermissions checks the permissions required to create an issue
// in the specified component.
func (bm *BugManager) checkComponentPermissions(ctx context.Context, componentID int64) (componentPermissions, error) {
	var err error
	permissions := componentPermissions{}
	permissions.appender, err = bm.checkSinglePermission(ctx, componentID, false, "appender")
	if err != nil {
		return permissions, err
	}
	permissions.issueDefaultsAppender, err = bm.checkSinglePermission(ctx, componentID, true, "appender")
	if err != nil {
		return permissions, err
	}
	return permissions, nil
}

// checkSinglePermission checks a single permission of a Buganizer component
// ID.  You should typically use checkComponentPermission instead of this
// method.
func (bm *BugManager) checkSinglePermission(ctx context.Context, componentID int64, issueDefaults bool, relation string) (bool, error) {
	resource := []string{"components", strconv.Itoa(int(componentID))}
	if issueDefaults {
		resource = append(resource, "issueDefaults")
	}
	automationAccessRequest := &issuetracker.GetAutomationAccessRequest{
		User:         &issuetracker.User{EmailAddress: bm.selfEmail},
		Relation:     relation,
		ResourceName: strings.Join(resource, "/"),
	}
	if bm.Simulate {
		logging.Debugf(ctx, "Would check Buganizer component permission: %s", textPBMultiline.Format(automationAccessRequest))
	} else {
		access, err := bm.client.GetAutomationAccess(ctx, automationAccessRequest)
		if err != nil {
			logging.Errorf(ctx, "error when checking buganizer component permissions with request:\n%s\nerror:%s", textPBMultiline.Format(automationAccessRequest), err)
			return false, err
		}
		return access.HasAccess, nil
	}
	return false, nil
}
