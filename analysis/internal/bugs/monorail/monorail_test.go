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
	"context"
	"testing"
	"time"

	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"

	mpb "go.chromium.org/luci/analysis/internal/bugs/monorail/api_proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestClient(t *testing.T) {
	t.Parallel()

	Convey("With Existing Issue Data", t, func() {
		issue1 := NewIssueData(1)
		issue2 := NewIssueData(2)
		issue3 := NewIssueData(3)
		f := &FakeIssuesStore{
			Issues: []*IssueData{issue1, issue2, issue3},
			NextID: 4,
		}
		ctx := UseFakeIssuesClient(context.Background(), f, "user@chromium.org")

		now := time.Date(2044, time.April, 4, 4, 4, 4, 4, time.UTC)
		ctx, _ = testclock.UseTime(ctx, now)

		Convey("Get issue", func() {
			c, err := NewClient(ctx, "monorailhost")
			So(err, ShouldBeNil)
			result, err := c.GetIssue(ctx, "projects/monorailproject/issues/1")
			So(err, ShouldBeNil)
			So(result, ShouldResembleProto, issue1.Issue)
		})
		Convey("Batch get issues", func() {
			c, err := NewClient(ctx, "monorailhost")
			So(err, ShouldBeNil)
			ids := []string{
				"1",
				"2",
				"4", // Does not exist.
				"1",
				"2",
				"3",
			}
			result, err := c.BatchGetIssues(ctx, "monorailproject", ids)
			So(err, ShouldBeNil)
			So(result, ShouldResembleProto, []*mpb.Issue{issue1.Issue, issue2.Issue, nil, issue1.Issue, issue2.Issue, issue3.Issue})
		})
		Convey("Make issue", func() {
			issue := NewIssue(4)
			issue.Name = ""
			req := &mpb.MakeIssueRequest{
				Parent:      "projects/monorailproject",
				Issue:       issue,
				Description: "Description",
				NotifyType:  mpb.NotifyType_NO_NOTIFICATION,
			}

			c, err := NewClient(ctx, "monorailhost")
			So(err, ShouldBeNil)
			result, err := c.MakeIssue(ctx, req)
			So(err, ShouldBeNil)
			expectedResult := NewIssue(4)
			expectedResult.StatusModifyTime = timestamppb.New(now)
			So(result, ShouldResembleProto, expectedResult)

			comments, err := c.ListComments(ctx, result.Name)
			So(err, ShouldBeNil)
			So(len(comments), ShouldEqual, 1)
			So(comments[0].Content, ShouldEqual, "Description")
		})
		Convey("List comments", func() {
			Convey("Single comment", func() {
				c, err := NewClient(ctx, "monorailhost")
				So(err, ShouldBeNil)
				comments, err := c.ListComments(ctx, "projects/monorailproject/issues/1")
				So(err, ShouldBeNil)
				So(len(comments), ShouldEqual, 1)
				So(comments, ShouldResembleProto, issue1.Comments)
			})
			Convey("Many comments", func() {
				issue := NewIssueData(4)
				for i := 2; i <= 3*maxCommentPageSize; i++ {
					issue.Comments = append(issue.Comments, NewComment(issue.Issue.Name, i))
				}
				f.Issues = append(f.Issues, issue)

				c, err := NewClient(ctx, "monorailhost")
				So(err, ShouldBeNil)
				comments, err := c.ListComments(ctx, issue.Issue.Name)
				So(err, ShouldBeNil)
				So(comments, ShouldResembleProto, issue.Comments)
			})
		})
		Convey("Modify issue", func() {
			issue1.Issue.Labels = []*mpb.Issue_LabelValue{
				{Label: "Test-Label1"},
			}

			c, err := NewClient(ctx, "monorailhost")
			So(err, ShouldBeNil)

			req := &mpb.ModifyIssuesRequest{
				Deltas: []*mpb.IssueDelta{
					{
						Issue: &mpb.Issue{
							Name:   issue1.Issue.Name,
							Status: &mpb.Issue_StatusValue{Status: VerifiedStatus},
							Labels: []*mpb.Issue_LabelValue{
								{
									Label: "Test-Label2",
								},
							},
						},
						UpdateMask: &field_mask.FieldMask{
							Paths: []string{"labels", "status"},
						},
					},
				},
				CommentContent: "Changing status and labels.",
			}
			err = c.ModifyIssues(ctx, req)
			So(err, ShouldBeNil)

			expectedData := NewIssueData(1)
			expectedData.Issue.Labels = []*mpb.Issue_LabelValue{
				{Label: "Test-Label1"},
				{Label: "Test-Label2"},
			}
			expectedData.Issue.Status = &mpb.Issue_StatusValue{Status: VerifiedStatus}
			expectedData.Issue.StatusModifyTime = timestamppb.New(now)

			read, err := c.GetIssue(ctx, issue1.Issue.Name)
			So(err, ShouldBeNil)
			So(read, ShouldResembleProto, expectedData.Issue)

			comments, err := c.ListComments(ctx, issue1.Issue.Name)
			So(err, ShouldBeNil)
			So(len(comments), ShouldEqual, 2)
			So(comments[0], ShouldResembleProto, expectedData.Comments[0])
			So(comments[1].Content, ShouldEqual, "Changing status and labels.")
		})
	})
}
