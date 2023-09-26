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

	"google.golang.org/protobuf/proto"

	mpb "go.chromium.org/luci/analysis/internal/bugs/monorail/api_proto"
)

// IssueData is a representation of all data stored for an issue, used by
// FakeIssuesClient.
type IssueData struct {
	Issue    *mpb.Issue
	Comments []*mpb.Comment
	// NotifyCount is the number of times a notification has been generated
	// for the issue.
	NotifyCount int
}

// FakeIssuesSystem stores the state of bugs for a fake implementation of monorail.
type FakeIssuesStore struct {
	Issues []*IssueData
	// Resource names of valid components.
	// E.g. projects/chromium/componentDefs/Blink>Workers.
	ComponentNames    []string
	NextID            int
	PriorityFieldName string
}

// NewIssueData creates new monorail issue data for testing.
func NewIssueData(uniqifier int) *IssueData {
	result := &IssueData{}
	result.Issue = NewIssue(uniqifier)
	result.Comments = []*mpb.Comment{
		NewComment(result.Issue.Name, 1),
	}
	result.NotifyCount = 0
	return result
}

// NewIssue returns a new monorail issue proto for testing.
func NewIssue(uniqifier int) *mpb.Issue {
	return &mpb.Issue{
		Name:    fmt.Sprintf("projects/monorailproject/issues/%v", uniqifier),
		Summary: fmt.Sprintf("This is the summary of bug %v.", uniqifier),
		State:   mpb.IssueContentState_ACTIVE,
		Status: &mpb.Issue_StatusValue{
			Status: UntriagedStatus,
		},
		Reporter: "user@chromium.org",
		FieldValues: []*mpb.FieldValue{
			{
				Field: ChromiumTestTypeField,
				Value: "Bug",
			},
			{
				Field: ChromiumTestPriorityField,
				Value: "1",
			},
		},
	}
}

// NewComment returns a new monorail comment proto for testing.
func NewComment(issueName string, number int) *mpb.Comment {
	return &mpb.Comment{
		Name:      fmt.Sprintf("%s/comment/%v", issueName, number),
		State:     mpb.IssueContentState_ACTIVE,
		Type:      mpb.Comment_DESCRIPTION,
		Content:   "Issue Description.",
		Commenter: "user@chromium.org",
	}
}

// CopyIssuesStore performs a deep copy of the given FakeIssuesStore.
func CopyIssuesStore(s *FakeIssuesStore) *FakeIssuesStore {
	var issues []*IssueData
	for _, iss := range s.Issues {
		issues = append(issues, CopyIssueData(iss))
	}
	return &FakeIssuesStore{
		Issues:            issues,
		ComponentNames:    append([]string{}, s.ComponentNames...),
		NextID:            s.NextID,
		PriorityFieldName: s.PriorityFieldName,
	}
}

// CopyIssuesStore performs a deep copy of the given IssueData.
func CopyIssueData(d *IssueData) *IssueData {
	return &IssueData{
		Issue:       CopyIssue(d.Issue),
		Comments:    CopyComments(d.Comments),
		NotifyCount: d.NotifyCount,
	}
}

// CopyIssue performs a deep copy of the given Issue.
func CopyIssue(issue *mpb.Issue) *mpb.Issue {
	result := &mpb.Issue{}
	proto.Merge(result, issue)
	return result
}

// CopyComments performs a deep copy of the given Comment.
func CopyComments(comments []*mpb.Comment) []*mpb.Comment {
	var result []*mpb.Comment
	for _, c := range comments {
		copy := &mpb.Comment{}
		proto.Merge(copy, c)
		result = append(result, copy)
	}
	return result
}
