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
	"strings"
	"testing"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/testmetadata"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

func TestQueryTestMetadata(t *testing.T) {
	ftt.Run(`QueryTestMetadata`, t, func(t *ftt.Test) {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm1", Permission: rdbperms.PermListTestMetadata},
				{Realm: "testproject:testrealm2", Permission: rdbperms.PermListTestMetadata},
			},
		})
		req := &pb.QueryTestMetadataRequest{
			Project:   "testproject",
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
		testutil.MustApply(ctx, t, insert.TestMetadataRows(append(expectedT1Rows, expectedT2Rows...))...)
		testutil.MustApply(ctx, t, insert.TestMetadataRows(otherRows)...)

		srv := newTestResultDBService()

		t.Run(`Permission denied`, func(t *ftt.Test) {
			res, err := srv.QueryTestMetadata(ctx, &pb.QueryTestMetadataRequest{Project: "x"})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("caller does not have permission resultdb.testMetadata.list in any realm in project \"x\""))
			assert.Loosely(t, res, should.BeNil)
		})

		t.Run(`Invalid request`, func(t *ftt.Test) {
			request := &pb.QueryTestMetadataRequest{
				Project: "testproject",
				Predicate: &pb.TestMetadataPredicate{
					TestIds: []string{"test"},
				},
			}
			t.Run("Invalid project name", func(t *ftt.Test) {
				request.Project = "testproject:testrealm1"
				res, err := srv.QueryTestMetadata(ctx, request)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike(`project: does not match pattern "^[a-z0-9\\-]{1,40}$"`))
				assert.Loosely(t, res, should.BeNil)
			})

			t.Run("Invalid page size", func(t *ftt.Test) {
				request.PageSize = -1
				res, err := srv.QueryTestMetadata(ctx, request)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike(`page_size`))
				assert.Loosely(t, res, should.BeNil)
			})

			t.Run("Predicate", func(t *ftt.Test) {
				t.Run("Invalid test ID", func(t *ftt.Test) {
					request.Predicate.TestIds = []string{strings.Repeat("a", 600)}
					_, err := srv.QueryTestMetadata(ctx, request)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike(`predicate: test_ids[0]: longer than 512 bytes`))
				})
				t.Run("Invalid previous test ID", func(t *ftt.Test) {
					request.Predicate.PreviousTestIds = []string{strings.Repeat("a", 600)}
					_, err := srv.QueryTestMetadata(ctx, request)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike(`predicate: previous_test_ids[0]: longer than 512 bytes`))
				})
				t.Run("Both test ID and previous test ID", func(t *ftt.Test) {
					request.Predicate.TestIds = []string{"test"}
					request.Predicate.PreviousTestIds = []string{"previous_test"}
					_, err := srv.QueryTestMetadata(ctx, request)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike(`predicate: either test_ids or previous_test_ids may be specified; not both`))
				})
			})
		})

		t.Run(`Valid request`, func(t *ftt.Test) {
			t.Run(`No predicate`, func(t *ftt.Test) {
				req.Predicate = nil
				res, err := srv.QueryTestMetadata(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res.NextPageToken, should.BeEmpty)
				expected := toTestMetadataDetails(append(expectedT1Rows, expectedT2Rows...))
				sortMetadata(expected)
				assert.Loosely(t, res.TestMetadata, should.Match(expected))
			})

			t.Run(`Filter on test ID`, func(t *ftt.Test) {
				req.Predicate = &pb.TestMetadataPredicate{
					TestIds: []string{"test1"},
				}
				t.Run(`Baseline`, func(t *ftt.Test) {
					res, err := srv.QueryTestMetadata(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, res.NextPageToken, should.BeEmpty)
					assert.Loosely(t, res.TestMetadata, should.Match(toTestMetadataDetails(expectedT1Rows)))
				})
				t.Run(`Try next page`, func(t *ftt.Test) {
					req.PageToken = pagination.Token("test1", hex.EncodeToString([]byte("hash1")))
					res, err := srv.QueryTestMetadata(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, res.NextPageToken, should.BeEmpty)
					assert.Loosely(t, res.TestMetadata, should.Match(toTestMetadataDetails(expectedT1Rows[1:])))
				})
			})
			t.Run(`Filter previous test ID`, func(t *ftt.Test) {
				req.Predicate = &pb.TestMetadataPredicate{
					PreviousTestIds: []string{"previous_id_for_test1"},
				}
				t.Run(`Baseline`, func(t *ftt.Test) {
					res, err := srv.QueryTestMetadata(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, res.NextPageToken, should.BeEmpty)
					assert.Loosely(t, res.TestMetadata, should.Match(toTestMetadataDetails(expectedT1Rows)))
				})
				t.Run(`Try next page`, func(t *ftt.Test) {
					req.PageToken = pagination.Token("test1", hex.EncodeToString([]byte("hash1")))
					res, err := srv.QueryTestMetadata(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, res.NextPageToken, should.BeEmpty)
					assert.Loosely(t, res.TestMetadata, should.Match(toTestMetadataDetails(expectedT1Rows[1:])))
				})
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
