// Copyright 2023 The LUCI Authors.
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

package resultdb

import (
	"encoding/hex"
	"sort"
	"testing"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/testmetadata"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestQueryTestMetadata(t *testing.T) {
	Convey(`QueryTestMetadata`, t, func() {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm1", Permission: rdbperms.PermListTestMetadata},
				{Realm: "testproject:testrealm2", Permission: rdbperms.PermListTestMetadata},
			},
		})
		req := &pb.QueryTestMetadataRequest{
			Project: "testproject",
			Predicate: &pb.TestMetadataPredicate{
				TestIds: []string{"test1"},
			},
			PageSize:  0, // Use default max page size.
			PageToken: "",
		}
		expectedT1Rows := []*testmetadata.TestMetadataRow{
			insert.MakeTestMetadataRow("testproject", "test1", "testrealm1", []byte("hash1")),
			insert.MakeTestMetadataRow("testproject", "test1", "testrealm2", []byte("hash2")),
		}
		expectedT2Rows := []*testmetadata.TestMetadataRow{
			insert.MakeTestMetadataRow("testproject", "test2", "testrealm1", []byte("hash1")),
		}
		otherRows := []*testmetadata.TestMetadataRow{
			insert.MakeTestMetadataRow("testproject", "test1", "testrealm2", []byte("hash1")),      // Duplicated row with allowed realm.
			insert.MakeTestMetadataRow("testprojectother", "test1", "testrealm1", []byte("hash1")), // Different project.
			insert.MakeTestMetadataRow("testprojectother", "test1", "testrealm3", []byte("hash3")), // Realm with no permission.
		}
		testutil.MustApply(ctx, insert.TestMetadataRows(append(expectedT1Rows, expectedT2Rows...))...)
		testutil.MustApply(ctx, insert.TestMetadataRows(otherRows)...)

		srv := newTestResultDBService()

		Convey(`Permission denied`, func() {
			res, err := srv.QueryTestMetadata(ctx, &pb.QueryTestMetadataRequest{Project: "x"})
			So(err, ShouldBeRPCPermissionDenied, "caller does not have permission resultdb.testMetadata.list in any realm in project \"x\"")
			So(res, ShouldBeNil)
		})

		Convey(`Invalid request`, func() {
			Convey("Invalid project name", func() {
				res, err := srv.QueryTestMetadata(ctx, &pb.QueryTestMetadataRequest{
					Project: "testproject:testrealm1",
					Predicate: &pb.TestMetadataPredicate{
						TestIds: []string{"test"},
					},
				})
				So(err, ShouldBeRPCInvalidArgument, `project: does not match pattern "^[a-z0-9\\-]{1,40}$"`)
				So(res, ShouldBeNil)
			})

			Convey("Invalid page size", func() {
				res, err := srv.QueryTestMetadata(ctx, &pb.QueryTestMetadataRequest{
					Project: "testproject",
					Predicate: &pb.TestMetadataPredicate{
						TestIds: []string{"test"},
					},
					PageSize: -1,
				})
				So(err, ShouldBeRPCInvalidArgument, `page_size`)
				So(res, ShouldBeNil)
			})
		})

		Convey(`Valid request`, func() {
			Convey(`No predicate`, func() {
				req.Predicate = nil

				res, err := srv.QueryTestMetadata(ctx, req)
				So(err, ShouldBeNil)
				So(res.NextPageToken, ShouldEqual, "")
				expected := toTestMetadataDetails(append(expectedT1Rows, expectedT2Rows...))
				sortMetadata(expected)
				So(res.TestMetadata, ShouldResembleProto, expected)

			})

			Convey(`Filter test id`, func() {
				res, err := srv.QueryTestMetadata(ctx, req)
				So(err, ShouldBeNil)
				So(res.NextPageToken, ShouldEqual, "")
				So(res.TestMetadata, ShouldResembleProto, toTestMetadataDetails(expectedT1Rows))
			})

			Convey(`Try next page`, func() {
				req.PageToken = pagination.Token("test1", hex.EncodeToString([]byte("hash1")))
				res, err := srv.QueryTestMetadata(ctx, req)
				So(err, ShouldBeNil)
				So(res.NextPageToken, ShouldEqual, "")
				So(res.TestMetadata, ShouldResembleProto, toTestMetadataDetails(expectedT1Rows[1:]))

			})
		})
	})
}

func toTestMetadataDetails(rows []*testmetadata.TestMetadataRow) (tmds []*pb.TestMetadataDetail) {
	for _, row := range rows {
		tmds = append(tmds, &pb.TestMetadataDetail{
			Name:         pbutil.TestMetadataName(row.Project, row.TestID, row.RefHash),
			Project:      row.Project,
			TestId:       row.TestID,
			RefHash:      hex.EncodeToString(row.RefHash),
			SourceRef:    row.SourceRef,
			TestMetadata: row.TestMetadata,
		})
	}
	return tmds
}

func sortMetadata(tmd []*pb.TestMetadataDetail) {
	combineKey := func(tmd *pb.TestMetadataDetail) string {
		return tmd.Project + "\n" + tmd.TestId + "\n" + tmd.RefHash
	}
	sort.Slice(tmd, func(i, j int) bool {
		return combineKey(tmd[i]) < combineKey(tmd[j])
	})
}
