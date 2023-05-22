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

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/analysis/internal/bugs"
	configpb "go.chromium.org/luci/analysis/proto/config"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/third_party/google.golang.org/genproto/googleapis/devtools/issuetracker/v1"
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
	// The GAE APP ID, e.g. "luci-analysis".
	appID string
	// The LUCI Project.
	project string
	// The snapshot of configuration to use for the project.
	projectCfg *configpb.ProjectConfig
	// The generator used to generate updates to Buganizer bugs.
	requestGenerator RequestGenerator

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
	appID, project string,
	projectCfg *configpb.ProjectConfig,
	simulate bool) *BugManager {
	requestGenerator := NewRequestGenerator(
		client,
		appID,
		project,
		projectCfg,
	)
	return &BugManager{
		client:           client,
		projectCfg:       projectCfg,
		appID:            appID,
		project:          project,
		requestGenerator: *requestGenerator,
		Simulate:         simulate,
	}
}

// Create creates an issue in Buganizer and returns the issue ID.
func (bm *BugManager) Create(ctx context.Context, createRequest *bugs.CreateRequest) (string, error) {
	componentID := bm.projectCfg.Buganizer.DefaultComponent.Id
	wantedComponentID := createRequest.BuganizerComponent
	if wantedComponentID != componentID && wantedComponentID > 0 {
		permissions, err := bm.checkComponentPermissions(ctx, wantedComponentID)
		if err != nil {
			return "", errors.Annotate(err, "check permissions to create Buganizer issue").Err()
		}
		if permissions.appender && permissions.issueDefaultsAppender {
			componentID = createRequest.BuganizerComponent
		}
	}
	createIssueRequest := bm.requestGenerator.PrepareNew(
		createRequest.Impact,
		createRequest.Description,
		componentID,
	)
	var issue *issuetracker.Issue
	var issueId int64
	var err error
	if bm.Simulate {
		logging.Debugf(ctx, "Would create Buganizer issue: %s", textPBMultiline.Format(createIssueRequest))
		issueId = 123456
	} else {
		issue, err = bm.client.CreateIssue(ctx, createIssueRequest)
		if err != nil {
			return "", errors.Annotate(err, "create Buganizer issue").Err()
		}
		issueId = issue.IssueId
	}

	if bm.Simulate {
		logging.Debugf(ctx, "Would update Buganizer issue and add issue link to description")
	} else {
		issueCommentReq := bm.requestGenerator.PrepareLinkIssueCommentUpdate(issue)
		if _, err := bm.client.UpdateIssueComment(ctx, issueCommentReq); err != nil {
			if statusError, ok := status.FromError(err); ok && statusError.Code() == codes.PermissionDenied {
				// If we fail to update the issue comment, then we add a comment with the issue link.
				logging.Warningf(ctx, "Failed to update issue comment: %v", statusError)
				issueCommentReq := bm.requestGenerator.PrepareLinkComment(issueId)
				if _, err := bm.client.CreateIssueComment(ctx, issueCommentReq); err != nil {
					return "", errors.Annotate(err, "create issue link comment").Err()
				}
			} else {
				return "", errors.Annotate(err, "add issue link to issue comment").Err()
			}
		}
	}

	if wantedComponentID > 0 && wantedComponentID != componentID {
		issueCommentReq := bm.requestGenerator.PrepareNoPermissionComment(issueId, wantedComponentID)
		if bm.Simulate {
			logging.Debugf(ctx, "Would update Buganizer issue: %s", textPBMultiline.Format(issueCommentReq))
		} else {
			if _, err := bm.client.CreateIssueComment(ctx, issueCommentReq); err != nil {
				return "", errors.Annotate(err, "create issue link comment").Err()
			}
		}
	}

	if bm.Simulate {
		return "", bugs.ErrCreateSimulated
	}

	bugs.BugsCreatedCounter.Add(ctx, 1, bm.project, "buganizer")
	return strconv.Itoa(int(issueId)), nil
}

// Update updates the issues in Buganizer.
func (bm *BugManager) Update(ctx context.Context, requests []bugs.BugUpdateRequest) ([]bugs.BugUpdateResponse, error) {
	issues, err := bm.fetchIssues(ctx, requests)
	if err != nil {
		return nil, errors.Annotate(err, "fetch issues for update").Err()
	}

	var responses []bugs.BugUpdateResponse

	issuesByID := make(map[int64]*issuetracker.Issue)

	for _, fetchedIssue := range issues {
		issuesByID[fetchedIssue.IssueId] = fetchedIssue
	}

	for _, request := range requests {
		id, err := strconv.ParseInt(request.Bug.ID, 10, 64)
		if err != nil {
			return nil, errors.Annotate(err, "convert bug id to int").Err()
		}
		issue, ok := issuesByID[id]
		if !ok {
			// The bug does not exist, or is in a different buganizer project
			// to the buganizer project configured for this project
			// or we have no permission to access it.
			//Take no action.
			responses = append(responses, bugs.BugUpdateResponse{
				IsDuplicate:   false,
				ShouldArchive: false,
			})
			logging.Warningf(ctx, "Buganizer issue %s not found or we don't have permission to access it, skipping.", request.Bug.ID)
			continue
		}
		updateResponse := bugs.BugUpdateResponse{
			IsDuplicate:                issue.IssueState.Status == issuetracker.Issue_DUPLICATE,
			ShouldArchive:              shouldArchiveRule(ctx, issue, request.IsManagingBug),
			DisableRulePriorityUpdates: false,
		}

		if !updateResponse.IsDuplicate &&
			!updateResponse.ShouldArchive &&
			request.IsManagingBug &&
			request.Impact != nil {
			if bm.requestGenerator.NeedsUpdate(request.Impact, issue, request.IsManagingBugPriority) {
				if err != nil {
					return nil, errors.Annotate(err, "read impact rule").Err()
				}
				mur, err := bm.requestGenerator.MakeUpdate(ctx, MakeUpdateOptions{
					impact:                           request.Impact,
					issue:                            issue,
					IsManagingBugPriority:            request.IsManagingBugPriority,
					IsManagingBugPriorityLastUpdated: request.IsManagingBugPriorityLastUpdated,
				})
				if err != nil {
					return nil, errors.Annotate(err, "create update request for issue").Err()
				}
				if bm.Simulate {
					logging.Debugf(ctx, "Would update Buganizer issue: %s", textPBMultiline.Format(mur.request))
				} else {
					if _, err := bm.client.ModifyIssue(ctx, mur.request); err != nil {
						return nil, errors.Annotate(err, "failed to update Buganizer issue %s", request.Bug.ID).Err()
					}
					bugs.BugsUpdatedCounter.Add(ctx, 1, bm.project, "buganizer")
				}
				updateResponse.DisableRulePriorityUpdates = mur.disablePriorityUpdates
			}
		}
		responses = append(responses, updateResponse)
	}

	return responses, nil
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
	if request.Bug.System != bugs.BuganizerSystem {
		// Indicates an implementation error with the caller.
		panic("Buganizer bug manager can only deal with Buganizer bugs")
	}
	issueId, err := strconv.Atoi(request.Bug.ID)
	if err != nil {
		return errors.Annotate(err, "update duplicate source").Err()
	}
	req := bm.requestGenerator.UpdateDuplicateSource(int64(issueId), request.ErrorMessage, request.DestinationRuleID)
	if bm.Simulate {
		logging.Debugf(ctx, "Would update Buganizer issue: %s", textPBMultiline.Format(req))
	} else {
		if _, err := bm.client.ModifyIssue(ctx, req); err != nil {
			return errors.Annotate(err, "failed to update duplicate source Buganizer issue %s", request.Bug.ID).Err()
		}
	}
	return nil
}

// UpdateDuplicateDestination updates the destination bug of a duplicate
// bug relationship.
// It posts a message advising the user LUCI Analysis
// has merged the rule for the source bug to the destination
// (merged-into) bug, and provides a link to the failure
// association rule.
func (bm *BugManager) UpdateDuplicateDestination(ctx context.Context, destinationBug bugs.BugID) error {
	if destinationBug.System != bugs.BuganizerSystem {
		// Indicates an implementation error with the caller.
		panic("Buganizer bug manager can only deal with Buganizer bugs")
	}
	issueId, err := strconv.Atoi(destinationBug.ID)
	if err != nil {
		return errors.Annotate(err, "update duplicate destination").Err()
	}
	req := bm.requestGenerator.UpdateDuplicateDestination(int64(issueId))
	if err != nil {
		return errors.Annotate(err, "mark issue as available").Err()
	}
	if bm.Simulate {
		logging.Debugf(ctx, "Would update Buganizer issue: %s", textPBMultiline.Format(req))
	} else {
		if _, err := bm.client.ModifyIssue(ctx, req); err != nil {
			return errors.Annotate(err, "failed to update duplicate destination Buganizer issue %s", destinationBug.ID).Err()
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
		User:         &issuetracker.User{EmailAddress: ctx.Value(&BuganizerSelfEmailKey).(string)},
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
