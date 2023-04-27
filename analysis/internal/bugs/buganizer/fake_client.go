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
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/third_party/google.golang.org/genproto/googleapis/devtools/issuetracker/v1"
)

// ComponentWithNoAccess is a componentID for which all access checks to the
// fake client will return false.
const ComponentWithNoAccess = 999999

// FakeClient is an implementation of ClientWrapperInterface that fakes the
// actions performed using an in-memory store.
type FakeClient struct {
	FakeStore *FakeIssueStore
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

func (fic *FakeClient) GetIssue(ctx context.Context, in *issuetracker.GetIssueRequest) (*issuetracker.Issue, error) {
	issueData, err := fic.FakeStore.GetIssue(in.IssueId)
	if err != nil {
		return nil, errors.Annotate(err, "fake get issue").Err()
	}
	return issueData.Issue, nil
}

// CreateIssue creates an issue in the in-memory store.
func (fic *FakeClient) CreateIssue(ctx context.Context, in *issuetracker.CreateIssueRequest) (*issuetracker.Issue, error) {
	return fic.FakeStore.StoreIssue(ctx, in.Issue), nil
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
	if issueData.ShouldFailUpdates {
		return nil, errors.New("issue is set to fail updates")
	}
	issue := issueData.Issue
	// The fields in the switch statement are the only
	// fields supported by the method.
	for _, addPath := range in.AddMask.Paths {
		switch addPath {
		case "status":
			if issue.IssueState.Status != in.Add.Status {
				issue.IssueState.Status = in.Add.Status
				issue.ModifiedTime = timestamppb.New(clock.Now(ctx))
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
			return nil, errors.New(fmt.Sprintf("unsupported issue field: %s", addPath))
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
	issueData, err := fic.FakeStore.GetIssue(in.IssueId)
	if err != nil {
		return nil, errors.Annotate(err, "fake create issue comment").Err()
	}
	in.Comment.IssueId = in.IssueId
	issueData.Comments = append(issueData.Comments, in.Comment)
	return in.Comment, nil
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
