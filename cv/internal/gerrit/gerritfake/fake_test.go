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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/cv/internal/gerrit"
)

func TestRelationship(t *testing.T) {
	t.Parallel()

	ftt.Run("Relationship works", t, func(t *ftt.Test) {
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

		t.Run("with allowed project", func(t *ftt.Test) {
			gc, err := f.MakeClient(ctx, "host", "infra")
			assert.NoErr(t, err)

			t.Run("No relations", func(t *ftt.Test) {
				resp, err := gc.GetRelatedChanges(ctx, &gerritpb.GetRelatedChangesRequest{
					Number:     4,
					Project:    "infra/infra",
					RevisionId: "1",
				})
				assert.NoErr(t, err)
				assert.That(t, resp, should.Match(&gerritpb.GetRelatedChangesResponse{}))
			})

			t.Run("Descendants only", func(t *ftt.Test) {
				resp, err := gc.GetRelatedChanges(ctx, &gerritpb.GetRelatedChangesRequest{
					Number:     2,
					Project:    "infra/infra",
					RevisionId: "1",
				})
				assert.NoErr(t, err)
				sortRelated(resp)
				assert.That(t, resp, should.Match(&gerritpb.GetRelatedChangesResponse{
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
				}))
			})

			t.Run("Diamond", func(t *ftt.Test) {
				resp, err := gc.GetRelatedChanges(ctx, &gerritpb.GetRelatedChangesRequest{
					Number:     4,
					RevisionId: "4",
				})
				assert.NoErr(t, err)
				sortRelated(resp)
				assert.That(t, resp, should.Match(&gerritpb.GetRelatedChangesResponse{
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
				}))
			})

			t.Run("Part of Diamond", func(t *ftt.Test) {
				resp, err := gc.GetRelatedChanges(ctx, &gerritpb.GetRelatedChangesRequest{
					Number:     3,
					RevisionId: "3",
				})
				assert.NoErr(t, err)
				sortRelated(resp)
				assert.That(t, resp, should.Match(&gerritpb.GetRelatedChangesResponse{
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
				}))
			})
		})

		t.Run("with disallowed project", func(t *ftt.Test) {
			gc, err := f.MakeClient(ctx, "host", "spying-luci-project")
			assert.NoErr(t, err)
			_, err = gc.GetRelatedChanges(ctx, &gerritpb.GetRelatedChangesRequest{
				Number:     4,
				RevisionId: "1",
			})
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.NotFound))
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

	ftt.Run("Files' handling works", t, func(t *ftt.Test) {
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
		assert.NoErr(t, err)

		t.Run("change or revision NotFound", func(t *ftt.Test) {
			_, err := gc.ListFiles(ctx, &gerritpb.ListFilesRequest{Number: 123213, RevisionId: "1"})
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.NotFound))
			_, err = gc.ListFiles(ctx, &gerritpb.ListFilesRequest{
				Number:     ciDefault.GetNumber(),
				RevisionId: "not existing",
			})
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.NotFound))
		})

		t.Run("Default", func(t *ftt.Test) {
			resp, err := gc.ListFiles(ctx, &gerritpb.ListFilesRequest{
				Number:     ciDefault.GetNumber(),
				RevisionId: ciDefault.GetCurrentRevision(),
			})
			assert.NoErr(t, err)
			assert.That(t, sortedFiles(resp), should.Match([]string{"ps001/c.cpp", "shared/s.py"}))
		})

		t.Run("Custom", func(t *ftt.Test) {
			resp, err := gc.ListFiles(ctx, &gerritpb.ListFilesRequest{
				Number:     ciCustom.GetNumber(),
				RevisionId: "1",
			})
			assert.NoErr(t, err)
			assert.That(t, sortedFiles(resp), should.Match([]string{"bl.ah", "ps1/cus.tom"}))
			resp, err = gc.ListFiles(ctx, &gerritpb.ListFilesRequest{
				Number:     ciCustom.GetNumber(),
				RevisionId: "2",
			})
			assert.NoErr(t, err)
			assert.That(t, sortedFiles(resp), should.Match([]string{"still/custom"}))
		})

		t.Run("NoFiles", func(t *ftt.Test) {
			resp, err := gc.ListFiles(ctx, &gerritpb.ListFilesRequest{
				Number:     ciNoFiles.GetNumber(),
				RevisionId: ciNoFiles.GetCurrentRevision(),
			})
			assert.NoErr(t, err)
			assert.Loosely(t, resp.GetFiles(), should.HaveLength(0))
		})
	})
}

func TestGetChange(t *testing.T) {
	t.Parallel()

	ftt.Run("GetChange handling works", t, func(t *ftt.Test) {
		ci := CI(100100, PS(4), AllRevs())
		assert.Loosely(t, ci.GetRevisions(), should.HaveLength(4))
		f := WithCIs("host", ACLRestricted("infra"), ci)

		ctx := context.Background()
		gc, err := f.MakeClient(ctx, "host", "infra")
		assert.NoErr(t, err)

		t.Run("NotFound", func(t *ftt.Test) {
			_, err := gc.GetChange(ctx, &gerritpb.GetChangeRequest{Number: 12321})
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.NotFound))
		})

		t.Run("Default", func(t *ftt.Test) {
			resp, err := gc.GetChange(ctx, &gerritpb.GetChangeRequest{Number: 100100})
			assert.NoErr(t, err)
			assert.Loosely(t, resp.GetCurrentRevision(), should.BeEmpty)
			assert.Loosely(t, resp.GetRevisions(), should.HaveLength(0))
			assert.Loosely(t, resp.GetLabels(), should.HaveLength(0))
		})

		t.Run("CURRENT_REVISION", func(t *ftt.Test) {
			resp, err := gc.GetChange(ctx, &gerritpb.GetChangeRequest{
				Number:  100100,
				Options: []gerritpb.QueryOption{gerritpb.QueryOption_CURRENT_REVISION}})
			assert.NoErr(t, err)
			assert.Loosely(t, resp.GetRevisions(), should.HaveLength(1))
			assert.Loosely(t, resp.GetRevisions()[resp.GetCurrentRevision()], should.NotBeNil)
		})

		t.Run("Full", func(t *ftt.Test) {
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
			assert.NoErr(t, err)
			assert.That(t, resp, should.Match(ci))
		})
	})
}

func TestListAccountEmails(t *testing.T) {
	t.Parallel()

	ftt.Run("ListAccountEmails works", t, func(t *ftt.Test) {

		ctx := context.Background()
		f := Fake{}
		f.AddLinkedAccountMapping([]*gerritpb.EmailInfo{
			{Email: "foo@google.com"},
			{Email: "foo@chromium.org"},
		})

		client, err := f.MakeClient(ctx, "foo", "bar")
		assert.NoErr(t, err)

		t.Run("returns linked email for the given email address", func(t *ftt.Test) {
			res, err := client.ListAccountEmails(ctx, &gerritpb.ListAccountEmailsRequest{
				Email: "foo@google.com",
			})

			assert.NoErr(t, err)
			assert.That(t, res, should.Match(&gerritpb.ListAccountEmailsResponse{
				Emails: []*gerritpb.EmailInfo{
					{Email: "foo@google.com"},
					{Email: "foo@chromium.org"},
				},
			}))
		})

		t.Run("returns error when the account doesn't exist", func(t *ftt.Test) {
			_, err := client.ListAccountEmails(ctx, &gerritpb.ListAccountEmailsRequest{
				Email: "bar@google.com",
			})
			assert.ErrIsLike(t, err, "Account 'bar@google.com' not found")
		})
	})
}

func TestListChanges(t *testing.T) {
	t.Parallel()

	ftt.Run("ListChanges works", t, func(t *ftt.Test) {
		f := WithCIs("empty", ACLRestricted("empty"))
		ctx := context.Background()

		mustCurrentClient := func(host, luciProject string) gerrit.Client {
			cl, err := f.MakeClient(ctx, host, luciProject)
			assert.NoErr(t, err)
			return cl
		}

		listChangeIDs := func(client gerrit.Client, req *gerritpb.ListChangesRequest) []int {
			out, err := client.ListChanges(ctx, req)
			assert.NoErr(t, err)
			assert.Loosely(t, out.GetMoreChanges(), should.BeFalse)
			ids := make([]int, len(out.GetChanges()))
			for i, ch := range out.GetChanges() {
				ids[i] = int(ch.GetNumber())
				if i > 0 {
					// Ensure monotonically non-decreasing update timestamps.
					prior := out.GetChanges()[i-1]
					assert.Loosely(t, prior.GetUpdated().AsTime().Before(ch.GetUpdated().AsTime()), should.BeFalse)
				}
			}
			return ids
		}

		f.AddFrom(WithCIs("chrome-internal", ACLRestricted("infra-internal"),
			CI(9001, Project("infra/infra-internal")),
			CI(9002, Project("infra/infra-internal")),
		))

		t.Run("ACLs enforced", func(t *ftt.Test) {
			assert.Loosely(t, listChangeIDs(mustCurrentClient("chrome-internal", "spy"),
				&gerritpb.ListChangesRequest{}), should.Match([]int{}))
			assert.Loosely(t, listChangeIDs(mustCurrentClient("chrome-internal", "infra-internal"),
				&gerritpb.ListChangesRequest{}), should.Match([]int{9002, 9001}))
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

		t.Run("Order and limit", func(t *ftt.Test) {
			g := mustCurrentClient("chromium", "anyone")
			assert.That(t, listChangeIDs(g, &gerritpb.ListChangesRequest{}), should.Match([]int{8002, 8001, 8003}))

			out, err := g.ListChanges(ctx, &gerritpb.ListChangesRequest{Limit: 2})
			assert.NoErr(t, err)
			assert.Loosely(t, out.GetMoreChanges(), should.BeTrue)
			assert.Loosely(t, out.GetChanges()[0].GetNumber(), should.Equal(8002))
			assert.Loosely(t, out.GetChanges()[1].GetNumber(), should.Equal(8001))
		})

		t.Run("Filtering works", func(t *ftt.Test) {
			query := func(q string) []int {
				return listChangeIDs(mustCurrentClient("chromium", "anyone"),
					&gerritpb.ListChangesRequest{Query: q})
			}
			t.Run("before/after", func(t *ftt.Test) {
				assert.That(t, gerritutil.FormatTime(epoch), should.Match(`"2011-02-03 04:05:06.000000007"`))
				assert.That(t, query(`before:"2011-02-03 04:05:06.000000006"`), should.Match([]int{}))
				// 1 ns later
				assert.That(t, query(`before:"2011-02-03 04:05:06.000000007"`), should.Match([]int{8003}))
				assert.That(t, query(` after:"2011-02-03 04:05:06.000000007"`), should.Match([]int{8002, 8001, 8003}))
				// 1 minute later
				assert.That(t, query(` after:"2011-02-03 04:06:06.000000007"`), should.Match([]int{8002, 8001}))
				// 1 minute later
				assert.That(t, query(` after:"2011-02-03 04:07:06.000000007"`), should.Match([]int{8002}))
				// Surround middle CL:
				assert.Loosely(t, query(``+
					` after:"2011-02-03 04:05:30.000000000" `+
					`before:"2011-02-03 04:06:30.000000000"`), should.Match([]int{8001}))
			})
			t.Run("Project prefix", func(t *ftt.Test) {
				assert.That(t, query(`projects:"inf"`), should.Match([]int{8002, 8001, 8003}))
				assert.That(t, query(`projects:"infra/"`), should.Match([]int{8002, 8001, 8003}))
				assert.That(t, query(`projects:"infra/luci"`), should.Match([]int{8002, 8003}))
				assert.That(t, query(`projects:"typo"`), should.Match([]int{}))
			})
			t.Run("Project exact", func(t *ftt.Test) {
				assert.That(t, query(`project:"infra/infra"`), should.Match([]int{8001}))
				assert.That(t, query(`project:"infra"`), should.Match([]int{}))
				assert.That(t, query(`(project:"infra/infra" OR project:"infra/luci/luci-go")`), should.Match(
					[]int{8002, 8001, 8003}))
			})
			t.Run("Status", func(t *ftt.Test) {
				assert.That(t, query(`status:new`), should.Match([]int{8002, 8001}))
				assert.That(t, query(`status:abandoned`), should.Match([]int{}))
				assert.That(t, query(`status:merged`), should.Match([]int{8003}))
			})
			t.Run("label", func(t *ftt.Test) {
				assert.That(t, query(`label:Commit-Queue>0`), should.Match([]int{8002, 8001}))
				assert.That(t, query(`label:Commit-Queue>1`), should.Match([]int{8001}))
				assert.That(t, query(`label:Code-Review>-1`), should.Match([]int{8003}))
			})
			t.Run("Typical CV query", func(t *ftt.Test) {
				assert.Loosely(t, query(`label:Commit-Queue>0 status:NEW project:"infra/infra"`),
					should.Match([]int{8001}))
				assert.Loosely(t, query(`label:Commit-Queue>0 status:NEW projects:"infra"`),
					should.Match([]int{8002, 8001}))
				assert.Loosely(t, query(`label:Commit-Queue>0 status:NEW projects:"infra"`+
					` after:"2011-02-03 04:06:30.000000000" `+
					`before:"2011-02-03 04:08:30.000000000"`), should.Match([]int{8002}))
				assert.Loosely(t, query(`label:Commit-Queue>0 status:NEW `+
					`(project:"infra" OR project:"infra/luci/luci-go")`+
					` after:"2011-02-03 04:06:30.000000000" `+
					`before:"2011-02-03 04:08:30.000000000"`), should.Match([]int{8002}))
			})
		})

		t.Run("Bad queries", func(t *ftt.Test) {
			test := func(query string) error {
				client, err := f.MakeClient(ctx, "infra", "chromium")
				assert.NoErr(t, err)
				_, err = client.ListChanges(ctx, &gerritpb.ListChangesRequest{Query: query})
				assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.InvalidArgument))
				assert.ErrIsLike(t, err, `invalid query argument`)
				return err
			}

			assert.ErrIsLike(t, test(`"unmatched quote`), `invalid query argument "\"unmatched quote"`)
			assert.ErrIsLike(t, test(`status:new "unmatched`), `unrecognized token "\"unmatched`)
			assert.ErrIsLike(t, test(`project:"unmatched`), `"project:\"unmatched": expected quoted string`)
			assert.ErrIsLike(t, test(`project:raw/not/supported`), `expected quoted string`)
			assert.ErrIsLike(t, test(`project:"one" OR project:"two"`), `"OR" must be inside ()`)
			assert.ErrIsLike(t, test(`project:"one" project:"two")`), `"project:" must be inside ()`)
			// This error can be better, but UX isn't essential for a fake.
			assert.ErrIsLike(t, test(`(project:"one" OR`), `"" must be outside of ()`)

			assert.ErrIsLike(t, test(`status:rand-om`), `unrecognized status "rand-om"`)
			assert.ErrIsLike(t, test(`status:0`), `unrecognized status "0"`)
			assert.ErrIsLike(t, test(`label:0`), `invalid label: 0`)
			assert.ErrIsLike(t, test(`label:Commit-Queue`), `invalid label: Commit-Queue`)

			// Note these are actually allowed in Gerrit.
			assert.ErrIsLike(t, test(`label:Commit-Queue<1`), `invalid label: Commit-Queue<1`)
			assert.ErrIsLike(t, test(`before:2019-20-01`), `failed to parse Gerrit timestamp "2019-20-01"`)
			assert.ErrIsLike(t, test(` after:2019-20-01`), `failed to parse Gerrit timestamp "2019-20-01"`)
			assert.ErrIsLike(t, test(`before:"2019-20-01"`), `failed to parse Gerrit timestamp "\"2019-20-01\""`)
		})
	})
}

func TestSetReview(t *testing.T) {
	t.Parallel()

	ftt.Run("SetReview", t, func(t *ftt.Test) {
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
			assert.NoErr(t, err)
			return cl
		}

		latestCI := func() *gerritpb.ChangeInfo {
			return f.GetChange("example", 10001).Info
		}
		t.Run("ACLs enforced", func(t *ftt.Test) {
			client := mustWriterClient("example", "not-chromium")
			res, err := client.SetReview(ctx, &gerritpb.SetReviewRequest{
				Number: 11111,
			})
			assert.Loosely(t, res, should.BeNil)
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.NotFound))

			res, err = client.SetReview(ctx, &gerritpb.SetReviewRequest{
				Number:  10001,
				Message: "this is a message",
			})
			assert.Loosely(t, res, should.BeNil)
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.PermissionDenied))

			res, err = client.SetReview(ctx, &gerritpb.SetReviewRequest{
				Number: 10001,
				Labels: map[string]int32{
					"Commit-Queue": 0,
				},
			})
			assert.Loosely(t, res, should.BeNil)
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.PermissionDenied))

			res, err = client.SetReview(ctx, &gerritpb.SetReviewRequest{
				Number: 10001,
				Labels: map[string]int32{
					"Commit-Queue": 0,
				},
				OnBehalfOf: accountID,
			})
			assert.Loosely(t, res, should.BeNil)
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.PermissionDenied))
		})

		t.Run("Post message", func(t *ftt.Test) {
			client := mustWriterClient("example", "chromium")
			res, err := client.SetReview(ctx, &gerritpb.SetReviewRequest{
				Number:  10001,
				Message: "this is a message",
			})
			assert.NoErr(t, err)
			assert.That(t, res, should.Match(&gerritpb.ReviewResult{}))
			assert.Loosely(t, latestCI().GetUpdated().AsTime(), should.HappenAfter(ciBefore.GetUpdated().AsTime()))
			assert.That(t, latestCI().GetMessages(), should.Match([]*gerritpb.ChangeMessageInfo{
				{
					Id:      "0",
					Author:  U("chromium"),
					Date:    timestamppb.New(tc.Now()),
					Message: "this is a message",
				},
			}))
		})

		t.Run("Set vote", func(t *ftt.Test) {
			client := mustWriterClient("example", "chromium")
			res, err := client.SetReview(ctx, &gerritpb.SetReviewRequest{
				Number: 10001,
				Labels: map[string]int32{
					"Commit-Queue": 2,
				},
			})
			assert.NoErr(t, err)
			assert.That(t, res, should.Match(&gerritpb.ReviewResult{
				Labels: map[string]int32{
					"Commit-Queue": 2,
				},
			}))
			assert.Loosely(t, latestCI().GetUpdated().AsTime(), should.HappenAfter(ciBefore.GetUpdated().AsTime()))
			assert.That(t, latestCI().GetLabels()["Commit-Queue"].GetAll(), should.Match([]*gerritpb.ApprovalInfo{
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
			}))
		})

		t.Run("Set vote on behalf of", func(t *ftt.Test) {
			client := mustWriterClient("example", "chromium")
			t.Run("existing voter", func(t *ftt.Test) {
				res, err := client.SetReview(ctx, &gerritpb.SetReviewRequest{
					Number: 10001,
					Labels: map[string]int32{
						"Commit-Queue": 0,
					},
					OnBehalfOf: 123,
				})
				assert.NoErr(t, err)
				assert.That(t, res, should.Match(&gerritpb.ReviewResult{
					Labels: map[string]int32{
						"Commit-Queue": 0,
					},
				}))
				assert.Loosely(t, latestCI().GetUpdated().AsTime(), should.HappenAfter(ciBefore.GetUpdated().AsTime()))
				assert.Loosely(t, NonZeroVotes(latestCI(), "Commit-Queue"), should.BeEmpty)
			})

			t.Run("new voter", func(t *ftt.Test) {
				res, err := client.SetReview(ctx, &gerritpb.SetReviewRequest{
					Number: 10001,
					Labels: map[string]int32{
						"Commit-Queue": 1,
					},
					OnBehalfOf: 789,
				})
				assert.NoErr(t, err)
				assert.That(t, res, should.Match(&gerritpb.ReviewResult{
					Labels: map[string]int32{
						"Commit-Queue": 1,
					},
				}))
				assert.Loosely(t, latestCI().GetUpdated().AsTime(), should.HappenAfter(ciBefore.GetUpdated().AsTime()))
				assert.That(t, latestCI().GetLabels()["Commit-Queue"].GetAll(), should.Match([]*gerritpb.ApprovalInfo{
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
				}))
			})
		})
	})
}

func TestSubmitRevision(t *testing.T) {
	t.Parallel()

	ftt.Run("SubmitRevision", t, func(t *ftt.Test) {
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
				assert.Loosely(t, latestCI.GetStatus(), should.Equal(s))
			}
		}

		mustWriterClient := func(host, luciProject string) gerrit.Client {
			cl, err := f.MakeClient(ctx, host, luciProject)
			assert.NoErr(t, err)
			return cl
		}

		t.Run("ACLs enforced", func(t *ftt.Test) {
			client := mustWriterClient(gHost, "not-chromium")
			_, err := client.SubmitRevision(ctx, &gerritpb.SubmitRevisionRequest{
				Number:     ciSingular.GetNumber(),
				RevisionId: ciSingular.GetCurrentRevision(),
			})
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.PermissionDenied))
			assertStatus(gerritpb.ChangeStatus_NEW, ciSingular)
		})

		t.Run("ACLs enforced with CL stack", func(t *ftt.Test) {
			f.MutateChange(gHost, int(ciStackBase.GetNumber()), func(c *Change) {
				c.ACLs = ACLGrant(OpSubmit, codes.PermissionDenied, "pretty-much-denied-to-everyone")
			})
			client := mustWriterClient(gHost, "chromium")
			_, err := client.SubmitRevision(ctx, &gerritpb.SubmitRevisionRequest{
				Number:     ciStackTop.GetNumber(),
				RevisionId: ciStackTop.GetCurrentRevision(),
			})
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.PermissionDenied))
			assertStatus(gerritpb.ChangeStatus_NEW, ciStackBase, ciStackMid, ciStackTop)
		})

		t.Run("Non-existent revision", func(t *ftt.Test) {
			client := mustWriterClient(gHost, "chromium")
			_, err := client.SubmitRevision(ctx, &gerritpb.SubmitRevisionRequest{
				Number:     ciSingular.GetNumber(),
				RevisionId: "non-existent",
			})
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.NotFound))
			assertStatus(gerritpb.ChangeStatus_NEW, ciSingular)
		})

		t.Run("Old revision", func(t *ftt.Test) {
			client := mustWriterClient(gHost, "chromium")
			var oldRev string
			for rev := range ciSingular.GetRevisions() {
				if rev != ciSingular.GetCurrentRevision() {
					oldRev = rev
					break
				}
			}
			assert.Loosely(t, oldRev, should.NotBeEmpty)
			_, err := client.SubmitRevision(ctx, &gerritpb.SubmitRevisionRequest{
				Number:     ciSingular.GetNumber(),
				RevisionId: oldRev,
			})
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.FailedPrecondition))
			assert.ErrIsLike(t, err, "is not current")
		})

		t.Run("Works", func(t *ftt.Test) {
			client := mustWriterClient(gHost, "chromium")
			res, err := client.SubmitRevision(ctx, &gerritpb.SubmitRevisionRequest{
				Number:     ciSingular.GetNumber(),
				RevisionId: ciSingular.GetCurrentRevision(),
			})
			assert.NoErr(t, err)
			assert.That(t, res, should.Match(&gerritpb.SubmitInfo{
				Status: gerritpb.ChangeStatus_MERGED,
			}))
			assertStatus(gerritpb.ChangeStatus_MERGED, ciSingular)
		})

		t.Run("Works with CL stack", func(t *ftt.Test) {
			client := mustWriterClient(gHost, "chromium")
			res, err := client.SubmitRevision(ctx, &gerritpb.SubmitRevisionRequest{
				Number:     ciStackTop.GetNumber(),
				RevisionId: ciStackTop.GetCurrentRevision(),
			})
			assert.NoErr(t, err)
			assert.That(t, res, should.Match(&gerritpb.SubmitInfo{
				Status: gerritpb.ChangeStatus_MERGED,
			}))
			assertStatus(gerritpb.ChangeStatus_MERGED, ciStackBase, ciStackMid, ciStackTop)
		})

		t.Run("Already Merged", func(t *ftt.Test) {
			f.MutateChange(gHost, int(ciSingular.GetNumber()), func(c *Change) {
				c.Info.Status = gerritpb.ChangeStatus_MERGED
			})
			client := mustWriterClient(gHost, "chromium")
			_, err := client.SubmitRevision(ctx, &gerritpb.SubmitRevisionRequest{
				Number:     ciSingular.GetNumber(),
				RevisionId: ciSingular.GetCurrentRevision(),
			})
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.FailedPrecondition))
			assert.ErrIsLike(t, err, "change is merged")
		})

		t.Run("already merged ones are skipped inside a CL Stack", func(t *ftt.Test) {
			verify := func() {
				client := mustWriterClient(gHost, "chromium")
				res, err := client.SubmitRevision(ctx, &gerritpb.SubmitRevisionRequest{
					Number:     ciStackTop.GetNumber(),
					RevisionId: ciStackTop.GetCurrentRevision(),
				})
				assert.NoErr(t, err)
				assert.That(t, res, should.Match(&gerritpb.SubmitInfo{
					Status: gerritpb.ChangeStatus_MERGED,
				}))
			}
			t.Run("base of stack", func(t *ftt.Test) {
				f.MutateChange(gHost, int(ciStackBase.GetNumber()), func(c *Change) {
					c.Info.Status = gerritpb.ChangeStatus_MERGED
				})
				verify()
				assertStatus(gerritpb.ChangeStatus_MERGED, ciStackBase, ciStackMid, ciStackTop)
			})
			t.Run("mid-stack", func(t *ftt.Test) {
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

		t.Run("Abandoned", func(t *ftt.Test) {
			f.MutateChange(gHost, int(ciSingular.GetNumber()), func(c *Change) {
				c.Info.Status = gerritpb.ChangeStatus_ABANDONED
			})
			client := mustWriterClient(gHost, "chromium")
			_, err := client.SubmitRevision(ctx, &gerritpb.SubmitRevisionRequest{
				Number:     ciSingular.GetNumber(),
				RevisionId: ciSingular.GetCurrentRevision(),
			})
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.FailedPrecondition))
			assert.ErrIsLike(t, err, "change is abandoned")
		})
		t.Run("Abandoned inside CL stack", func(t *ftt.Test) {
			f.MutateChange(gHost, int(ciStackMid.GetNumber()), func(c *Change) {
				c.Info.Status = gerritpb.ChangeStatus_ABANDONED
			})
			client := mustWriterClient(gHost, "chromium")
			_, err := client.SubmitRevision(ctx, &gerritpb.SubmitRevisionRequest{
				Number:     ciStackTop.GetNumber(),
				RevisionId: ciStackTop.GetCurrentRevision(),
			})
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.FailedPrecondition))
			assert.ErrIsLike(t, err, "change is abandoned")
		})
	})
}
