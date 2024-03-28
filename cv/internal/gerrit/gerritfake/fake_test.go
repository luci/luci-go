// Copyright 2020 The LUCI Authors.
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

package gerritfake

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	gerritutil "go.chromium.org/luci/common/api/gerrit"
	"go.chromium.org/luci/common/clock/testclock"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/cv/internal/gerrit"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestRelationship(t *testing.T) {
	t.Parallel()

	Convey("Relationship works", t, func() {
		ci1 := CI(1, PS(1), AllRevs())
		ci2 := CI(2, PS(2), AllRevs())
		ci3 := CI(3, PS(3), AllRevs())
		ci4 := CI(4, PS(4), AllRevs())
		f := WithCIs("host", ACLRestricted("infra"), ci1, ci2, ci3, ci4)
		// Diamond using latest patchsets.
		//      --<-- 2_2 --<--
		//     /               \
		//  1_1                 4_4
		//     \               /
		//      --<-- 3-3 --<--
		f.SetDependsOn("host", ci4, ci3, ci2) // 2 parents.
		f.SetDependsOn("host", ci3, ci1)
		f.SetDependsOn("host", ci2, ci1)

		// Chain made by prior patchsets.
		//  2_1 --<-- 3_2 --<-- 4_3
		f.SetDependsOn("host", "4_3", "3_2")
		f.SetDependsOn("host", "3_2", "2_1")
		ctx := context.Background()

		Convey("with allowed project", func() {
			gc, err := f.MakeClient(ctx, "host", "infra")
			So(err, ShouldBeNil)

			Convey("No relations", func() {
				resp, err := gc.GetRelatedChanges(ctx, &gerritpb.GetRelatedChangesRequest{
					Number:     4,
					Project:    "infra/infra",
					RevisionId: "1",
				})
				So(err, ShouldBeNil)
				So(resp, ShouldResembleProto, &gerritpb.GetRelatedChangesResponse{})
			})

			Convey("Descendants only", func() {
				resp, err := gc.GetRelatedChanges(ctx, &gerritpb.GetRelatedChangesRequest{
					Number:     2,
					Project:    "infra/infra",
					RevisionId: "1",
				})
				So(err, ShouldBeNil)
				sortRelated(resp)
				So(resp, ShouldResembleProto, &gerritpb.GetRelatedChangesResponse{
					Changes: []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit{
						{
							Project: "infra/infra",
							Commit: &gerritpb.CommitInfo{
								Id:      "rev-000002-001",
								Parents: []*gerritpb.CommitInfo_Parent{{Id: "fake_parent_commit"}},
							},
							Number:          2,
							Patchset:        1,
							CurrentPatchset: 2,
						},
						{
							Project: "infra/infra",
							Commit: &gerritpb.CommitInfo{
								Id:      "rev-000003-002",
								Parents: []*gerritpb.CommitInfo_Parent{{Id: "rev-000002-001"}},
							},
							Number:          3,
							Patchset:        2,
							CurrentPatchset: 3,
						},
						{
							Project: "infra/infra",
							Commit: &gerritpb.CommitInfo{
								Id:      "rev-000004-003",
								Parents: []*gerritpb.CommitInfo_Parent{{Id: "rev-000003-002"}},
							},
							Number:          4,
							Patchset:        3,
							CurrentPatchset: 4,
						},
					},
				})
			})

			Convey("Diamond", func() {
				resp, err := gc.GetRelatedChanges(ctx, &gerritpb.GetRelatedChangesRequest{
					Number:     4,
					RevisionId: "4",
				})
				So(err, ShouldBeNil)
				sortRelated(resp)
				So(resp, ShouldResembleProto, &gerritpb.GetRelatedChangesResponse{
					Changes: []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit{
						{
							Project: "infra/infra",
							Commit: &gerritpb.CommitInfo{
								Id:      "rev-000001-001",
								Parents: []*gerritpb.CommitInfo_Parent{{Id: "fake_parent_commit"}},
							},
							Number:          1,
							Patchset:        1,
							CurrentPatchset: 1,
						},
						{
							Project: "infra/infra",
							Commit: &gerritpb.CommitInfo{
								Id:      "rev-000002-002",
								Parents: []*gerritpb.CommitInfo_Parent{{Id: "rev-000001-001"}},
							},
							Number:          2,
							Patchset:        2,
							CurrentPatchset: 2,
						},
						{
							Project: "infra/infra",
							Commit: &gerritpb.CommitInfo{
								Id:      "rev-000003-003",
								Parents: []*gerritpb.CommitInfo_Parent{{Id: "rev-000001-001"}},
							},
							Number:          3,
							Patchset:        3,
							CurrentPatchset: 3,
						},
						{
							Project: "infra/infra",
							Commit: &gerritpb.CommitInfo{
								Id: "rev-000004-004",
								Parents: []*gerritpb.CommitInfo_Parent{
									{Id: "rev-000003-003"},
									{Id: "rev-000002-002"},
								},
							},
							Number:          4,
							Patchset:        4,
							CurrentPatchset: 4,
						},
					},
				})
			})

			Convey("Part of Diamond", func() {
				resp, err := gc.GetRelatedChanges(ctx, &gerritpb.GetRelatedChangesRequest{
					Number:     3,
					RevisionId: "3",
				})
				So(err, ShouldBeNil)
				sortRelated(resp)
				So(resp, ShouldResembleProto, &gerritpb.GetRelatedChangesResponse{
					Changes: []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit{
						{
							Project: "infra/infra",
							Commit: &gerritpb.CommitInfo{
								Id:      "rev-000001-001",
								Parents: []*gerritpb.CommitInfo_Parent{{Id: "fake_parent_commit"}},
							},
							Number:          1,
							Patchset:        1,
							CurrentPatchset: 1,
						},
						{
							Project: "infra/infra",
							Commit: &gerritpb.CommitInfo{
								Id:      "rev-000003-003",
								Parents: []*gerritpb.CommitInfo_Parent{{Id: "rev-000001-001"}},
							},
							Number:          3,
							Patchset:        3,
							CurrentPatchset: 3,
						},
						{
							Project: "infra/infra",
							Commit: &gerritpb.CommitInfo{
								Id: "rev-000004-004",
								Parents: []*gerritpb.CommitInfo_Parent{
									{Id: "rev-000003-003"},
									{Id: "rev-000002-002"},
								},
							},
							Number:          4,
							Patchset:        4,
							CurrentPatchset: 4,
						},
					},
				})
			})
		})

		Convey("with disallowed project", func() {
			gc, err := f.MakeClient(ctx, "host", "spying-luci-project")
			So(err, ShouldBeNil)
			_, err = gc.GetRelatedChanges(ctx, &gerritpb.GetRelatedChangesRequest{
				Number:     4,
				RevisionId: "1",
			})
			So(err, ShouldNotBeNil)
			So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
		})
	})
}

// sortRelated ensures deterministic yet ultimately abitrary order.
func sortRelated(r *gerritpb.GetRelatedChangesResponse) {
	key := func(i int) string {
		c := r.GetChanges()[i]
		return fmt.Sprintf("%40s:%020d:%020d", c.GetCommit().GetId(), c.GetNumber(), c.GetPatchset())
	}
	sort.Slice(r.GetChanges(), func(i, j int) bool { return key(i) < key(j) })
}

func TestFiles(t *testing.T) {
	t.Parallel()

	Convey("Files' handling works", t, func() {
		sortedFiles := func(r *gerritpb.ListFilesResponse) []string {
			fs := make([]string, 0, len(r.GetFiles()))
			for f := range r.GetFiles() {
				fs = append(fs, f)
			}
			sort.Strings(fs)
			return fs
		}
		ciDefault := CI(1)
		ciCustom := CI(2, Files("ps1/cus.tom", "bl.ah"), PS(2), Files("still/custom"))
		ciNoFiles := CI(3, Files())
		f := WithCIs("host", ACLRestricted("infra"), ciDefault, ciCustom, ciNoFiles)

		ctx := context.Background()
		gc, err := f.MakeClient(ctx, "host", "infra")
		So(err, ShouldBeNil)

		Convey("change or revision NotFound", func() {
			_, err := gc.ListFiles(ctx, &gerritpb.ListFilesRequest{Number: 123213, RevisionId: "1"})
			So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
			_, err = gc.ListFiles(ctx, &gerritpb.ListFilesRequest{
				Number:     ciDefault.GetNumber(),
				RevisionId: "not existing",
			})
			So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
		})

		Convey("Default", func() {
			resp, err := gc.ListFiles(ctx, &gerritpb.ListFilesRequest{
				Number:     ciDefault.GetNumber(),
				RevisionId: ciDefault.GetCurrentRevision(),
			})
			So(err, ShouldBeNil)
			So(sortedFiles(resp), ShouldResemble, []string{"ps001/c.cpp", "shared/s.py"})
		})

		Convey("Custom", func() {
			resp, err := gc.ListFiles(ctx, &gerritpb.ListFilesRequest{
				Number:     ciCustom.GetNumber(),
				RevisionId: "1",
			})
			So(err, ShouldBeNil)
			So(sortedFiles(resp), ShouldResemble, []string{"bl.ah", "ps1/cus.tom"})
			resp, err = gc.ListFiles(ctx, &gerritpb.ListFilesRequest{
				Number:     ciCustom.GetNumber(),
				RevisionId: "2",
			})
			So(err, ShouldBeNil)
			So(sortedFiles(resp), ShouldResemble, []string{"still/custom"})
		})

		Convey("NoFiles", func() {
			resp, err := gc.ListFiles(ctx, &gerritpb.ListFilesRequest{
				Number:     ciNoFiles.GetNumber(),
				RevisionId: ciNoFiles.GetCurrentRevision(),
			})
			So(err, ShouldBeNil)
			So(resp.GetFiles(), ShouldHaveLength, 0)
		})
	})
}

func TestGetChange(t *testing.T) {
	t.Parallel()

	Convey("GetChange handling works", t, func() {
		ci := CI(100100, PS(4), AllRevs())
		So(ci.GetRevisions(), ShouldHaveLength, 4)
		f := WithCIs("host", ACLRestricted("infra"), ci)

		ctx := context.Background()
		gc, err := f.MakeClient(ctx, "host", "infra")
		So(err, ShouldBeNil)

		Convey("NotFound", func() {
			_, err := gc.GetChange(ctx, &gerritpb.GetChangeRequest{Number: 12321})
			So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
		})

		Convey("Default", func() {
			resp, err := gc.GetChange(ctx, &gerritpb.GetChangeRequest{Number: 100100})
			So(err, ShouldBeNil)
			So(resp.GetCurrentRevision(), ShouldEqual, "")
			So(resp.GetRevisions(), ShouldHaveLength, 0)
			So(resp.GetLabels(), ShouldHaveLength, 0)
		})

		Convey("CURRENT_REVISION", func() {
			resp, err := gc.GetChange(ctx, &gerritpb.GetChangeRequest{
				Number:  100100,
				Options: []gerritpb.QueryOption{gerritpb.QueryOption_CURRENT_REVISION}})
			So(err, ShouldBeNil)
			So(resp.GetRevisions(), ShouldHaveLength, 1)
			So(resp.GetRevisions()[resp.GetCurrentRevision()], ShouldNotBeNil)
		})

		Convey("Full", func() {
			resp, err := gc.GetChange(ctx, &gerritpb.GetChangeRequest{
				Number: 100100,
				Options: []gerritpb.QueryOption{
					gerritpb.QueryOption_ALL_REVISIONS,
					gerritpb.QueryOption_DETAILED_ACCOUNTS,
					gerritpb.QueryOption_DETAILED_LABELS,
					gerritpb.QueryOption_SKIP_MERGEABLE,
					gerritpb.QueryOption_MESSAGES,
					gerritpb.QueryOption_SUBMITTABLE,
				}})
			So(err, ShouldBeNil)
			So(resp, ShouldResembleProto, ci)
		})
	})
}

func TestListAccountEmails(t *testing.T) {
	t.Parallel()

	Convey("ListAccountEmails works", t, func() {

		ctx := context.Background()
		f := Fake{}
		f.AddLinkedAccountMapping([]*gerritpb.EmailInfo{
			&gerritpb.EmailInfo{Email: "foo@google.com"},
			&gerritpb.EmailInfo{Email: "foo@chromium.org"},
		})

		client, err := f.MakeClient(ctx, "foo", "bar")
		So(err, ShouldBeNil)

		Convey("returns linked email for the given email address", func() {
			res, err := client.ListAccountEmails(ctx, &gerritpb.ListAccountEmailsRequest{
				Email: "foo@google.com",
			})

			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &gerritpb.ListAccountEmailsResponse{
				Emails: []*gerritpb.EmailInfo{
					&gerritpb.EmailInfo{Email: "foo@google.com"},
					&gerritpb.EmailInfo{Email: "foo@chromium.org"},
				},
			})
		})

		Convey("returns error when the account doesn't exist", func() {
			_, err := client.ListAccountEmails(ctx, &gerritpb.ListAccountEmailsRequest{
				Email: "bar@google.com",
			})
			So(err, ShouldErrLike, "Account 'bar@google.com' not found")
		})
	})
}

func TestListChanges(t *testing.T) {
	t.Parallel()

	Convey("ListChanges works", t, func() {
		f := WithCIs("empty", ACLRestricted("empty"))
		ctx := context.Background()

		mustCurrentClient := func(host, luciProject string) gerrit.Client {
			cl, err := f.MakeClient(ctx, host, luciProject)
			So(err, ShouldBeNil)
			return cl
		}

		listChangeIDs := func(client gerrit.Client, req *gerritpb.ListChangesRequest) []int {
			out, err := client.ListChanges(ctx, req)
			So(err, ShouldBeNil)
			So(out.GetMoreChanges(), ShouldBeFalse)
			ids := make([]int, len(out.GetChanges()))
			for i, ch := range out.GetChanges() {
				ids[i] = int(ch.GetNumber())
				if i > 0 {
					// Ensure monotonically non-decreasing update timestamps.
					prior := out.GetChanges()[i-1]
					So(prior.GetUpdated().AsTime().Before(ch.GetUpdated().AsTime()), ShouldBeFalse)
				}
			}
			return ids
		}

		f.AddFrom(WithCIs("chrome-internal", ACLRestricted("infra-internal"),
			CI(9001, Project("infra/infra-internal")),
			CI(9002, Project("infra/infra-internal")),
		))

		Convey("ACLs enforced", func() {
			So(listChangeIDs(mustCurrentClient("chrome-internal", "spy"),
				&gerritpb.ListChangesRequest{}), ShouldResemble, []int{})
			So(listChangeIDs(mustCurrentClient("chrome-internal", "infra-internal"),
				&gerritpb.ListChangesRequest{}), ShouldResemble, []int{9002, 9001})
		})

		var epoch = time.Date(2011, time.February, 3, 4, 5, 6, 7, time.UTC)
		u0 := Updated(epoch)
		u1 := Updated(epoch.Add(time.Minute))
		u2 := Updated(epoch.Add(2 * time.Minute))
		f.AddFrom(WithCIs("chromium", ACLPublic(),
			CI(8001, u1, Project("infra/infra"), CQ(+2)),
			CI(8002, u2, Project("infra/luci/luci-go"), Vote("Commit-Queue", +1), Vote("Code-Review", -1)),
			CI(8003, u0, Project("infra/luci/luci-go"), Status("MERGED"), Vote("Code-Review", +1)),
		))

		Convey("Order and limit", func() {
			g := mustCurrentClient("chromium", "anyone")
			So(listChangeIDs(g, &gerritpb.ListChangesRequest{}), ShouldResemble, []int{8002, 8001, 8003})

			out, err := g.ListChanges(ctx, &gerritpb.ListChangesRequest{Limit: 2})
			So(err, ShouldBeNil)
			So(out.GetMoreChanges(), ShouldBeTrue)
			So(out.GetChanges()[0].GetNumber(), ShouldEqual, 8002)
			So(out.GetChanges()[1].GetNumber(), ShouldEqual, 8001)
		})

		Convey("Filtering works", func() {
			query := func(q string) []int {
				return listChangeIDs(mustCurrentClient("chromium", "anyone"),
					&gerritpb.ListChangesRequest{Query: q})
			}
			Convey("before/after", func() {
				So(gerritutil.FormatTime(epoch), ShouldResemble, `"2011-02-03 04:05:06.000000007"`)
				So(query(`before:"2011-02-03 04:05:06.000000006"`), ShouldResemble, []int{})
				// 1 ns later
				So(query(`before:"2011-02-03 04:05:06.000000007"`), ShouldResemble, []int{8003})
				So(query(` after:"2011-02-03 04:05:06.000000007"`), ShouldResemble, []int{8002, 8001, 8003})
				// 1 minute later
				So(query(` after:"2011-02-03 04:06:06.000000007"`), ShouldResemble, []int{8002, 8001})
				// 1 minute later
				So(query(` after:"2011-02-03 04:07:06.000000007"`), ShouldResemble, []int{8002})
				// Surround middle CL:
				So(query(``+
					` after:"2011-02-03 04:05:30.000000000" `+
					`before:"2011-02-03 04:06:30.000000000"`), ShouldResemble, []int{8001})
			})
			Convey("Project prefix", func() {
				So(query(`projects:"inf"`), ShouldResemble, []int{8002, 8001, 8003})
				So(query(`projects:"infra/"`), ShouldResemble, []int{8002, 8001, 8003})
				So(query(`projects:"infra/luci"`), ShouldResemble, []int{8002, 8003})
				So(query(`projects:"typo"`), ShouldResemble, []int{})
			})
			Convey("Project exact", func() {
				So(query(`project:"infra/infra"`), ShouldResemble, []int{8001})
				So(query(`project:"infra"`), ShouldResemble, []int{})
				So(query(`(project:"infra/infra" OR project:"infra/luci/luci-go")`), ShouldResemble,
					[]int{8002, 8001, 8003})
			})
			Convey("Status", func() {
				So(query(`status:new`), ShouldResemble, []int{8002, 8001})
				So(query(`status:abandoned`), ShouldResemble, []int{})
				So(query(`status:merged`), ShouldResemble, []int{8003})
			})
			Convey("label", func() {
				So(query(`label:Commit-Queue>0`), ShouldResemble, []int{8002, 8001})
				So(query(`label:Commit-Queue>1`), ShouldResemble, []int{8001})
				So(query(`label:Code-Review>-1`), ShouldResemble, []int{8003})
			})
			Convey("Typical CV query", func() {
				So(query(`label:Commit-Queue>0 status:NEW project:"infra/infra"`),
					ShouldResemble, []int{8001})
				So(query(`label:Commit-Queue>0 status:NEW projects:"infra"`),
					ShouldResemble, []int{8002, 8001})
				So(query(`label:Commit-Queue>0 status:NEW projects:"infra"`+
					` after:"2011-02-03 04:06:30.000000000" `+
					`before:"2011-02-03 04:08:30.000000000"`), ShouldResemble, []int{8002})
				So(query(`label:Commit-Queue>0 status:NEW `+
					`(project:"infra" OR project:"infra/luci/luci-go")`+
					` after:"2011-02-03 04:06:30.000000000" `+
					`before:"2011-02-03 04:08:30.000000000"`), ShouldResemble, []int{8002})
			})
		})

		Convey("Bad queries", func() {
			test := func(query string) error {
				client, err := f.MakeClient(ctx, "infra", "chromium")
				So(err, ShouldBeNil)
				_, err = client.ListChanges(ctx, &gerritpb.ListChangesRequest{Query: query})
				So(grpcutil.Code(err), ShouldEqual, codes.InvalidArgument)
				So(err, ShouldErrLike, `invalid query argument`)
				return err
			}

			So(test(`"unmatched quote`), ShouldErrLike, `invalid query argument "\"unmatched quote"`)
			So(test(`status:new "unmatched`), ShouldErrLike, `unrecognized token "\"unmatched`)
			So(test(`project:"unmatched`), ShouldErrLike, `"project:\"unmatched": expected quoted string`)
			So(test(`project:raw/not/supported`), ShouldErrLike, `expected quoted string`)
			So(test(`project:"one" OR project:"two"`), ShouldErrLike, `"OR" must be inside ()`)
			So(test(`project:"one" project:"two")`), ShouldErrLike, `"project:" must be inside ()`)
			// This error can be better, but UX isn't essential for a fake.
			So(test(`(project:"one" OR`), ShouldErrLike, `"" must be outside of ()`)

			So(test(`status:rand-om`), ShouldErrLike, `unrecognized status "rand-om"`)
			So(test(`status:0`), ShouldErrLike, `unrecognized status "0"`)
			So(test(`label:0`), ShouldErrLike, `invalid label: 0`)
			So(test(`label:Commit-Queue`), ShouldErrLike, `invalid label: Commit-Queue`)

			// Note these are actually allowed in Gerrit.
			So(test(`label:Commit-Queue<1`), ShouldErrLike, `invalid label: Commit-Queue<1`)
			So(test(`before:2019-20-01`), ShouldErrLike, `failed to parse Gerrit timestamp "2019-20-01"`)
			So(test(` after:2019-20-01`), ShouldErrLike, `failed to parse Gerrit timestamp "2019-20-01"`)
			So(test(`before:"2019-20-01"`), ShouldErrLike, `failed to parse Gerrit timestamp "\"2019-20-01\""`)
		})
	})
}

func TestSetReview(t *testing.T) {
	t.Parallel()

	Convey("SetReview", t, func() {
		ctx, tc := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
		user := U("user-123")
		accountID := user.AccountId
		before := tc.Now().Add(-1 * time.Minute)
		ciBefore := CI(10001, CQ(1, before, user), Updated(before))
		f := WithCIs(
			"example",
			ACLGrant(OpReview, codes.PermissionDenied, "chromium").Or(ACLGrant(OpAlterVotesOfOthers, codes.PermissionDenied, "chromium")),
			ciBefore,
		)
		tc.Add(2 * time.Minute)

		mustWriterClient := func(host, luciProject string) gerrit.Client {
			cl, err := f.MakeClient(ctx, host, luciProject)
			So(err, ShouldBeNil)
			return cl
		}

		latestCI := func() *gerritpb.ChangeInfo {
			return f.GetChange("example", 10001).Info
		}
		Convey("ACLs enforced", func() {
			client := mustWriterClient("example", "not-chromium")
			res, err := client.SetReview(ctx, &gerritpb.SetReviewRequest{
				Number: 11111,
			})
			So(res, ShouldBeNil)
			So(grpcutil.Code(err), ShouldEqual, codes.NotFound)

			res, err = client.SetReview(ctx, &gerritpb.SetReviewRequest{
				Number:  10001,
				Message: "this is a message",
			})
			So(res, ShouldBeNil)
			So(grpcutil.Code(err), ShouldEqual, codes.PermissionDenied)

			res, err = client.SetReview(ctx, &gerritpb.SetReviewRequest{
				Number: 10001,
				Labels: map[string]int32{
					"Commit-Queue": 0,
				},
			})
			So(res, ShouldBeNil)
			So(grpcutil.Code(err), ShouldEqual, codes.PermissionDenied)

			res, err = client.SetReview(ctx, &gerritpb.SetReviewRequest{
				Number: 10001,
				Labels: map[string]int32{
					"Commit-Queue": 0,
				},
				OnBehalfOf: accountID,
			})
			So(res, ShouldBeNil)
			So(grpcutil.Code(err), ShouldEqual, codes.PermissionDenied)
		})

		Convey("Post message", func() {
			client := mustWriterClient("example", "chromium")
			res, err := client.SetReview(ctx, &gerritpb.SetReviewRequest{
				Number:  10001,
				Message: "this is a message",
			})
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &gerritpb.ReviewResult{})
			So(latestCI().GetUpdated().AsTime(), ShouldHappenAfter, ciBefore.GetUpdated().AsTime())
			So(latestCI().GetMessages(), ShouldResembleProto, []*gerritpb.ChangeMessageInfo{
				{
					Id:      "0",
					Author:  U("chromium"),
					Date:    timestamppb.New(tc.Now()),
					Message: "this is a message",
				},
			})
		})

		Convey("Set vote", func() {
			client := mustWriterClient("example", "chromium")
			res, err := client.SetReview(ctx, &gerritpb.SetReviewRequest{
				Number: 10001,
				Labels: map[string]int32{
					"Commit-Queue": 2,
				},
			})
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &gerritpb.ReviewResult{
				Labels: map[string]int32{
					"Commit-Queue": 2,
				},
			})
			So(latestCI().GetUpdated().AsTime(), ShouldHappenAfter, ciBefore.GetUpdated().AsTime())
			So(latestCI().GetLabels()["Commit-Queue"].GetAll(), ShouldResembleProto, []*gerritpb.ApprovalInfo{
				{
					User:  user,
					Value: 1,
					Date:  timestamppb.New(before),
				},
				{
					User:  U("chromium"),
					Value: 2,
					Date:  timestamppb.New(tc.Now()),
				},
			})
		})

		Convey("Set vote on behalf of", func() {
			client := mustWriterClient("example", "chromium")
			Convey("existing voter", func() {
				res, err := client.SetReview(ctx, &gerritpb.SetReviewRequest{
					Number: 10001,
					Labels: map[string]int32{
						"Commit-Queue": 0,
					},
					OnBehalfOf: 123,
				})
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &gerritpb.ReviewResult{
					Labels: map[string]int32{
						"Commit-Queue": 0,
					},
				})
				So(latestCI().GetUpdated().AsTime(), ShouldHappenAfter, ciBefore.GetUpdated().AsTime())
				So(NonZeroVotes(latestCI(), "Commit-Queue"), ShouldBeEmpty)
			})

			Convey("new voter", func() {
				res, err := client.SetReview(ctx, &gerritpb.SetReviewRequest{
					Number: 10001,
					Labels: map[string]int32{
						"Commit-Queue": 1,
					},
					OnBehalfOf: 789,
				})
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &gerritpb.ReviewResult{
					Labels: map[string]int32{
						"Commit-Queue": 1,
					},
				})
				So(latestCI().GetUpdated().AsTime(), ShouldHappenAfter, ciBefore.GetUpdated().AsTime())
				So(latestCI().GetLabels()["Commit-Queue"].GetAll(), ShouldResembleProto, []*gerritpb.ApprovalInfo{
					{
						User:  user,
						Value: 1,
						Date:  timestamppb.New(before),
					},
					{
						User:  U("user-789"),
						Value: 1,
						Date:  timestamppb.New(tc.Now()),
					},
				})
			})
		})
	})
}

func TestSubmitRevision(t *testing.T) {
	t.Parallel()

	Convey("SubmitRevision", t, func() {
		const gHost = "example.com"
		ctx, tc := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
		var (
			ciSingular  = CI(10001, Updated(tc.Now()), PS(3), AllRevs())
			ciStackBase = CI(20001, Updated(tc.Now()), PS(1), AllRevs())
			ciStackMid  = CI(20002, Updated(tc.Now()), PS(1), AllRevs())
			ciStackTop  = CI(20003, Updated(tc.Now()), PS(1), AllRevs())
		)
		f := WithCIs(
			"example.com",
			ACLGrant(OpSubmit, codes.PermissionDenied, "chromium"),
			ciSingular, ciStackBase, ciStackMid, ciStackTop,
		)
		f.SetDependsOn(gHost, ciStackMid, ciStackBase)
		f.SetDependsOn(gHost, ciStackTop, ciStackMid)

		tc.Add(2 * time.Minute)

		assertStatus := func(s gerritpb.ChangeStatus, cis ...*gerritpb.ChangeInfo) {
			for _, ci := range cis {
				latestCI := f.GetChange(gHost, int(ci.GetNumber())).Info
				So(latestCI.GetStatus(), ShouldEqual, s)
			}
		}

		mustWriterClient := func(host, luciProject string) gerrit.Client {
			cl, err := f.MakeClient(ctx, host, luciProject)
			So(err, ShouldBeNil)
			return cl
		}

		Convey("ACLs enforced", func() {
			client := mustWriterClient(gHost, "not-chromium")
			_, err := client.SubmitRevision(ctx, &gerritpb.SubmitRevisionRequest{
				Number:     ciSingular.GetNumber(),
				RevisionId: ciSingular.GetCurrentRevision(),
			})
			So(grpcutil.Code(err), ShouldEqual, codes.PermissionDenied)
			assertStatus(gerritpb.ChangeStatus_NEW, ciSingular)
		})

		Convey("ACLs enforced with CL stack", func() {
			f.MutateChange(gHost, int(ciStackBase.GetNumber()), func(c *Change) {
				c.ACLs = ACLGrant(OpSubmit, codes.PermissionDenied, "pretty-much-denied-to-everyone")
			})
			client := mustWriterClient(gHost, "chromium")
			_, err := client.SubmitRevision(ctx, &gerritpb.SubmitRevisionRequest{
				Number:     ciStackTop.GetNumber(),
				RevisionId: ciStackTop.GetCurrentRevision(),
			})
			So(grpcutil.Code(err), ShouldEqual, codes.PermissionDenied)
			assertStatus(gerritpb.ChangeStatus_NEW, ciStackBase, ciStackMid, ciStackTop)
		})

		Convey("Non-existent revision", func() {
			client := mustWriterClient(gHost, "chromium")
			_, err := client.SubmitRevision(ctx, &gerritpb.SubmitRevisionRequest{
				Number:     ciSingular.GetNumber(),
				RevisionId: "non-existent",
			})
			So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
			assertStatus(gerritpb.ChangeStatus_NEW, ciSingular)
		})

		Convey("Old revision", func() {
			client := mustWriterClient(gHost, "chromium")
			var oldRev string
			for rev := range ciSingular.GetRevisions() {
				if rev != ciSingular.GetCurrentRevision() {
					oldRev = rev
					break
				}
			}
			So(oldRev, ShouldNotBeEmpty)
			_, err := client.SubmitRevision(ctx, &gerritpb.SubmitRevisionRequest{
				Number:     ciSingular.GetNumber(),
				RevisionId: oldRev,
			})
			So(grpcutil.Code(err), ShouldEqual, codes.FailedPrecondition)
			So(err, ShouldErrLike, "is not current")
		})

		Convey("Works", func() {
			client := mustWriterClient(gHost, "chromium")
			res, err := client.SubmitRevision(ctx, &gerritpb.SubmitRevisionRequest{
				Number:     ciSingular.GetNumber(),
				RevisionId: ciSingular.GetCurrentRevision(),
			})
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &gerritpb.SubmitInfo{
				Status: gerritpb.ChangeStatus_MERGED,
			})
			assertStatus(gerritpb.ChangeStatus_MERGED, ciSingular)
		})

		Convey("Works with CL stack", func() {
			client := mustWriterClient(gHost, "chromium")
			res, err := client.SubmitRevision(ctx, &gerritpb.SubmitRevisionRequest{
				Number:     ciStackTop.GetNumber(),
				RevisionId: ciStackTop.GetCurrentRevision(),
			})
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &gerritpb.SubmitInfo{
				Status: gerritpb.ChangeStatus_MERGED,
			})
			assertStatus(gerritpb.ChangeStatus_MERGED, ciStackBase, ciStackMid, ciStackTop)
		})

		Convey("Already Merged", func() {
			f.MutateChange(gHost, int(ciSingular.GetNumber()), func(c *Change) {
				c.Info.Status = gerritpb.ChangeStatus_MERGED
			})
			client := mustWriterClient(gHost, "chromium")
			_, err := client.SubmitRevision(ctx, &gerritpb.SubmitRevisionRequest{
				Number:     ciSingular.GetNumber(),
				RevisionId: ciSingular.GetCurrentRevision(),
			})
			So(grpcutil.Code(err), ShouldEqual, codes.FailedPrecondition)
			So(err, ShouldErrLike, "change is merged")
		})

		Convey("already merged ones are skipped inside a CL Stack", func() {
			verify := func() {
				client := mustWriterClient(gHost, "chromium")
				res, err := client.SubmitRevision(ctx, &gerritpb.SubmitRevisionRequest{
					Number:     ciStackTop.GetNumber(),
					RevisionId: ciStackTop.GetCurrentRevision(),
				})
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &gerritpb.SubmitInfo{
					Status: gerritpb.ChangeStatus_MERGED,
				})
			}
			Convey("base of stack", func() {
				f.MutateChange(gHost, int(ciStackBase.GetNumber()), func(c *Change) {
					c.Info.Status = gerritpb.ChangeStatus_MERGED
				})
				verify()
				assertStatus(gerritpb.ChangeStatus_MERGED, ciStackBase, ciStackMid, ciStackTop)
			})
			Convey("mid-stack", func() {
				// May happen if the mid-CL was at some point re-based on top of
				// something other than ciStackBase and then submitted.
				f.MutateChange(gHost, int(ciStackMid.GetNumber()), func(c *Change) {
					c.Info.Status = gerritpb.ChangeStatus_MERGED
					PS(int(c.Info.GetRevisions()[c.Info.GetCurrentRevision()].GetNumber() + 1))(c.Info)
				})
				verify()
				assertStatus(gerritpb.ChangeStatus_MERGED, ciStackMid, ciStackTop)
				assertStatus(gerritpb.ChangeStatus_NEW, ciStackBase)
			})
		})

		Convey("Abandoned", func() {
			f.MutateChange(gHost, int(ciSingular.GetNumber()), func(c *Change) {
				c.Info.Status = gerritpb.ChangeStatus_ABANDONED
			})
			client := mustWriterClient(gHost, "chromium")
			_, err := client.SubmitRevision(ctx, &gerritpb.SubmitRevisionRequest{
				Number:     ciSingular.GetNumber(),
				RevisionId: ciSingular.GetCurrentRevision(),
			})
			So(grpcutil.Code(err), ShouldEqual, codes.FailedPrecondition)
			So(err, ShouldErrLike, "change is abandoned")
		})
		Convey("Abandoned inside CL stack", func() {
			f.MutateChange(gHost, int(ciStackMid.GetNumber()), func(c *Change) {
				c.Info.Status = gerritpb.ChangeStatus_ABANDONED
			})
			client := mustWriterClient(gHost, "chromium")
			_, err := client.SubmitRevision(ctx, &gerritpb.SubmitRevisionRequest{
				Number:     ciStackTop.GetNumber(),
				RevisionId: ciStackTop.GetCurrentRevision(),
			})
			So(grpcutil.Code(err), ShouldEqual, codes.FailedPrecondition)
			So(err, ShouldErrLike, "change is abandoned")
		})
	})
}
