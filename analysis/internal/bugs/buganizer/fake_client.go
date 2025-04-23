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
	"strconv"
	"strings"

	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/third_party/google.golang.org/genproto/googleapis/devtools/issuetracker/v1"
)

// ComponentWithNoAccess is a componentID for which all access checks to the
// fake client will return false.
const ComponentWithNoAccess = 999999

// ComponentIDWithArchived is a componentID for which GetComponent will return
// that the component is archived and for which CreateIssue will fail.
const ComponentWithIsArchivedSet = 999998

// FakeClient is an implementation of ClientWrapperInterface that fakes the
// actions performed using an in-memory store.
type FakeClient struct {
	FakeStore *FakeIssueStore
	// Defines a custom error to return when attempting to create
	// an issue comment. Use this to test failed updates.
	CreateCommentError error
	// Defines the component access level. Defaults to `AccessLimit_INTERNAL`.
	ComponentAccessLevel issuetracker.AccessLimit_AccessLevel
}

func NewFakeClient() *FakeClient {
	issueStore := NewFakeIssueStore()
	return &FakeClient{
		FakeStore: issueStore,
	}
}

// Required by interface but doesn't perform any closer in the fake state.
func (fic *FakeClient) Close() {
}

func (fic *FakeClient) BatchGetIssues(ctx context.Context, in *issuetracker.BatchGetIssuesRequest) (*issuetracker.BatchGetIssuesResponse, error) {
	issues, err := fic.FakeStore.BatchGetIssues(in.IssueIds)
	if err != nil {
		return nil, errors.Annotate(err, "fake batch get issues").Err()
	}
	return &issuetracker.BatchGetIssuesResponse{
		Issues: issues,
	}, nil
}

func (fic *FakeClient) GetComponent(ctx context.Context, in *issuetracker.GetComponentRequest) (*issuetracker.Component, error) {
	accessLevel := fic.ComponentAccessLevel
	if accessLevel == issuetracker.AccessLimit_ACCESS_LEVEL_UNSPECIFIED {
		accessLevel = issuetracker.AccessLimit_INTERNAL
	}

	return &issuetracker.Component{
		ComponentId: in.ComponentId,
		AccessLimit: &issuetracker.AccessLimit{
			AccessLevel: accessLevel,
		},
		IsArchived: in.ComponentId == ComponentWithIsArchivedSet,
	}, nil
}

func (fic *FakeClient) GetIssue(ctx context.Context, in *issuetracker.GetIssueRequest) (*issuetracker.Issue, error) {
	issueData, err := fic.FakeStore.GetIssue(in.IssueId)
	if err != nil {
		return nil, errors.Annotate(err, "fake get issue").Err()
	}
	if issueData.ShouldReturnAccessPermissionError {
		return nil, status.Error(codes.PermissionDenied, "cannot access bug")
	}
	return issueData.Issue, nil
}

// CreateIssue creates an issue in the in-memory store.
func (fic *FakeClient) CreateIssue(ctx context.Context, in *issuetracker.CreateIssueRequest) (*issuetracker.Issue, error) {
	if in.Issue.IssueId != 0 {
		return nil, errors.New("cannot set IssueId in CreateIssue requests")
	}
	if in.Issue.IssueState.ComponentId == ComponentWithNoAccess {
		return nil, errors.New("no access to component")
	}
	if in.Issue.IssueState.ComponentId == ComponentWithIsArchivedSet {
		return nil, errors.New("component is archived")
	}

	// Copy the request to make sure the proto we store
	// does not alias the request.
	issue := proto.Clone(in.Issue).(*issuetracker.Issue)

	// Move the issue description from IssueComment (the input-only field)
	// to Description (the output-only field).
	issue.Description = &issuetracker.IssueComment{
		CommentNumber: 1,
		Comment:       issue.IssueComment.Comment,
	}
	issue.IssueComment = nil

	if in.TemplateOptions != nil && in.TemplateOptions.ApplyTemplate {
		issue.IssueState.Ccs = []*issuetracker.User{
			{
				EmailAddress: "testcc1@google.com",
			},
			{
				EmailAddress: "testcc2@google.com",
			},
		}
	}

	return fic.FakeStore.StoreIssue(ctx, issue), nil
}

// GetAutomationAccess checks access to a ComponentID. Access is always true
// except for component ID ComponentWithNoAccess which is false.
func (fic *FakeClient) GetAutomationAccess(ctx context.Context, in *issuetracker.GetAutomationAccessRequest) (*issuetracker.GetAutomationAccessResponse, error) {
	return &issuetracker.GetAutomationAccessResponse{
		HasAccess: !strings.Contains(in.ResourceName, strconv.Itoa(ComponentWithNoAccess)),
	}, nil
}

// ModifyIssue modifies and issue in the in-memory store.
// This method handles a specific set of updates,
// please check the implementation and add any
// required field to the set of known fields.
func (fic *FakeClient) ModifyIssue(ctx context.Context, in *issuetracker.ModifyIssueRequest) (*issuetracker.Issue, error) {
	issueData, err := fic.FakeStore.GetIssue(in.IssueId)
	if err != nil {
		return nil, errors.Annotate(err, "fake modify issue").Err()
	}
	if issueData.UpdateError != nil {
		return nil, issueData.UpdateError
	}
	if issueData.ShouldReturnAccessPermissionError {
		return nil, status.Error(codes.PermissionDenied, "cannot access bug")
	}
	issue := issueData.Issue
	// The fields in the switch statement are the only
	// fields supported by the method.
	for _, addPath := range in.AddMask.Paths {
		switch addPath {
		case "status":
			if issue.IssueState.Status != in.Add.Status {
				issue.IssueState.Status = in.Add.Status

				now := timestamppb.New(clock.Now(ctx))
				issue.ModifiedTime = now
				if _, ok := ClosedStatuses[in.Add.Status]; ok {
					if !issue.ResolvedTime.IsValid() {
						// Resolved time is zero or unset. Set it.
						issue.ResolvedTime = now
					}
				} else {
					issue.ResolvedTime = nil
				}
				if in.Add.Status == issuetracker.Issue_VERIFIED {
					if !issue.VerifiedTime.IsValid() {
						// Verified time is zero or unset. Set it.
						issue.VerifiedTime = now
					}
				} else {
					issue.VerifiedTime = nil
				}
			}
		case "priority":
			if issue.IssueState.Priority != in.Add.Priority {
				issue.IssueState.Priority = in.Add.Priority
				issue.ModifiedTime = timestamppb.New(clock.Now(ctx))
			}
		case "verifier":
			if issue.IssueState.Verifier != in.Add.Verifier {
				issue.IssueState.Verifier = in.Add.Verifier
				issue.ModifiedTime = timestamppb.New(clock.Now(ctx))
			}
		case "assignee":
			if issue.IssueState.Assignee != in.Add.Assignee {
				issue.IssueState.Assignee = in.Add.Assignee
				issue.ModifiedTime = timestamppb.New(clock.Now(ctx))
			}
		default:
			return nil, errors.New(fmt.Sprintf("add_mask uses unsupported issue field: %s", addPath))
		}
	}
	// The fields in the switch statement are the only
	// fields supported by the method.
	for _, removePath := range in.RemoveMask.Paths {
		switch removePath {
		case "assignee":
			if in.Remove.Assignee != nil && in.Remove.Assignee.EmailAddress == "" {
				issue.IssueState.Assignee = nil
				issue.ModifiedTime = timestamppb.New(clock.Now(ctx))
			}
		default:
			return nil, errors.New(fmt.Sprintf("remove_mask uses unsupported issue field: %s", removePath))
		}
	}

	if in.IssueComment != nil {
		issueData.Comments = append(issueData.Comments, in.IssueComment)
	}
	return issue, nil
}

func (fic *FakeClient) ListIssueUpdates(ctx context.Context, in *issuetracker.ListIssueUpdatesRequest) IssueUpdateIterator {
	issueUpdates, err := fic.FakeStore.ListIssueUpdates(in.IssueId)
	if err != nil {
		return &fakeIssueUpdateIterator{
			err: errors.Annotate(err, "fake list issue updates").Err(),
		}
	}
	return &fakeIssueUpdateIterator{
		items: issueUpdates,
	}
}

func (fic *FakeClient) CreateIssueComment(ctx context.Context, in *issuetracker.CreateIssueCommentRequest) (*issuetracker.IssueComment, error) {
	if fic.CreateCommentError != nil {
		return nil, fic.CreateCommentError
	}
	issueData, err := fic.FakeStore.GetIssue(in.IssueId)
	if err != nil {
		return nil, errors.Annotate(err, "fake create issue comment").Err()
	}
	in.Comment.IssueId = in.IssueId
	issueData.Comments = append(issueData.Comments, in.Comment)
	return in.Comment, nil
}

func (fic *FakeClient) UpdateIssueComment(ctx context.Context, in *issuetracker.UpdateIssueCommentRequest) (*issuetracker.IssueComment, error) {
	issueData, err := fic.FakeStore.GetIssue(in.IssueId)
	if err != nil {
		return nil, errors.Annotate(err, "fake update issue comment").Err()
	}
	if in.CommentNumber < 1 || int(in.CommentNumber) > len(issueData.Comments) {
		return nil, errors.New("comment number is out of bounds")
	}
	comment := issueData.Comments[in.CommentNumber-1]
	comment.Comment = in.Comment.Comment
	issueData.Issue.Description = comment
	return comment, nil
}

func (fic *FakeClient) ListIssueComments(ctx context.Context, in *issuetracker.ListIssueCommentsRequest) IssueCommentIterator {
	issueData, err := fic.FakeStore.GetIssue(in.IssueId)
	if err != nil {
		return &fakeIssueCommentIterator{
			err: errors.Annotate(err, "fake list issue comments").Err(),
		}
	}

	return &fakeIssueCommentIterator{
		items: issueData.Comments,
	}
}

func (fic *FakeClient) CreateHotlistEntry(ctx context.Context, in *issuetracker.CreateHotlistEntryRequest) (*issuetracker.HotlistEntry, error) {
	err := fic.FakeStore.CreateHotlistEntry(in.HotlistEntry.IssueId, in.HotlistId)
	if err != nil {
		return nil, errors.Annotate(err, "fake create hotlist entry").Err()
	}
	return &issuetracker.HotlistEntry{
		IssueId:  in.HotlistEntry.IssueId,
		Position: 0, // Not currently populated by fake implementation.
	}, nil
}

type fakeIssueUpdateIterator struct {
	pointer int
	err     error
	items   []*issuetracker.IssueUpdate
}

func (fui *fakeIssueUpdateIterator) Next() (*issuetracker.IssueUpdate, error) {
	if fui.err != nil {
		return nil, fui.err
	}

	if fui.pointer == len(fui.items) {
		fui.err = iterator.Done
		return nil, fui.err
	}

	currentIndex := fui.pointer
	fui.pointer++
	return fui.items[currentIndex], nil
}

type fakeIssueCommentIterator struct {
	pointer int
	err     error
	items   []*issuetracker.IssueComment
}

func (fci *fakeIssueCommentIterator) Next() (*issuetracker.IssueComment, error) {
	if fci.err != nil {
		return nil, fci.err
	}

	if fci.pointer == len(fci.items) {
		fci.err = iterator.Done
		return nil, fci.err
	}
	currentIndex := fci.pointer
	fci.pointer++
	return fci.items[currentIndex], nil
}
