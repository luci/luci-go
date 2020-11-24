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

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/grpc/grpcutil"
	"google.golang.org/grpc/codes"

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
		ctx := f.Install(context.Background())

		Convey("with allowed project", func() {
			gc, err := gerrit.CurrentClient(ctx, "host", "infra")
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
								Id: "rev-000002-001",
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
								Id: "rev-000001-001",
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
								Id: "rev-000001-001",
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
			gc, err := gerrit.CurrentClient(ctx, "host", "spying-luci-project")
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

		ctx := f.Install(context.Background())
		gc, err := gerrit.CurrentClient(ctx, "host", "infra")
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
