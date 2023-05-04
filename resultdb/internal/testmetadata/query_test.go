// Copyright 2019 The LUCI Authors.
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

package testmetadata

import (
	"encoding/hex"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/span"
	"google.golang.org/grpc/codes"
)

func TestQueryTestMetadata(t *testing.T) {
	Convey(`Query`, t, func() {

		ctx := testutil.SpannerTestContext(t)
		q := &Query{
			Project:   "testproject",
			Predicate: &pb.TestMetadataPredicate{TestIds: []string{"test1"}},
			SubRealms: []string{"testrealm1", "testrealm2"},
			PageSize:  100,
		}
		fetch := func(q *Query) (trs []*pb.TestMetadataDetail, token string, err error) {
			ctx, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()
			return q.Fetch(ctx)
		}
		mustFetch := func(q *Query) (trs []*pb.TestMetadataDetail, token string) {
			trs, token, err := fetch(q)
			So(err, ShouldBeNil)
			return
		}

		Convey(`Returns correct rows`, func() {
			otherProjectRow := makeTestMetadataRow("otherProject", "test1", "testrealm1", []byte{uint8(1)})
			otherTestRow := makeTestMetadataRow("testproject", "othertest", "testrealm1", []byte{uint8(2)})
			noPermRealmRow := makeTestMetadataRow("testproject", "test1", "testrealm3", []byte{uint8(4)})
			expectedRow1 := makeTestMetadataRow("testproject", "test1", "testrealm1", []byte{uint8(0)})
			expectedRow2 := makeTestMetadataRow("testproject", "test1", "testrealm2", []byte{uint8(3)})

			testutil.MustApply(ctx, insertTestMetadataRows([]*TestMetadataRow{expectedRow1, otherProjectRow, otherTestRow, expectedRow2, noPermRealmRow})...)

			actual, token := mustFetch(q)
			So(token, ShouldEqual, "")
			So(actual, ShouldResembleProto, toTestMetadataDetails([]*TestMetadataRow{expectedRow1, expectedRow2}))
		})

		Convey(`Paging`, func() {
			makeTestMetadataWithSubRealm := func(subRealm string, size int) []*TestMetadataRow {
				rows := make([]*TestMetadataRow, size)
				for i := range rows {
					rows[i] = makeTestMetadataRow("testproject", "test1", subRealm, []byte{uint8(i)})
				}
				return rows
			}
			realm1Rows := makeTestMetadataWithSubRealm("testrealm1", 5)
			realm2Rows := makeTestMetadataWithSubRealm("testrealm2", 7) // 2 more rows with different refHash.

			testutil.MustApply(ctx, insertTestMetadataRows(realm1Rows)...)
			testutil.MustApply(ctx, insertTestMetadataRows(realm2Rows)...)

			mustReadPage := func(pageToken string, pageSize int, expected []*pb.TestMetadataDetail) string {
				q2 := q
				q2.PageToken = pageToken
				q2.PageSize = pageSize
				actual, token := mustFetch(q2)
				So(actual, ShouldResembleProto, expected)
				return token
			}

			Convey(`All results`, func() {
				token := mustReadPage("", 8, toTestMetadataDetails(append(realm1Rows, realm2Rows[5:]...)))
				So(token, ShouldEqual, "")
			})

			Convey(`With pagination`, func() {
				token := mustReadPage("", 1, toTestMetadataDetails(realm1Rows[:1])) // From lower subRealm.
				So(token, ShouldNotEqual, "")

				token = mustReadPage(token, 4, toTestMetadataDetails(realm1Rows[1:5])) // From lower subRealm.
				So(token, ShouldNotEqual, "")

				token = mustReadPage(token, 2, toTestMetadataDetails(realm2Rows[5:])) // From higher subReam.
				So(token, ShouldNotEqual, "")

				token = mustReadPage(token, 1, nil)
				So(token, ShouldEqual, "")
			})

			Convey(`Bad token`, func() {
				ctx, cancel := span.ReadOnlyTransaction(ctx)
				defer cancel()

				Convey(`From bad position`, func() {
					q.PageToken = "CgVoZWxsbw=="
					_, _, err := q.Fetch(ctx)
					So(err, ShouldHaveAppStatus, codes.InvalidArgument, "invalid page_token")
				})

				Convey(`From decoding`, func() {
					q.PageToken = "%%%"
					_, _, err := q.Fetch(ctx)
					So(err, ShouldHaveAppStatus, codes.InvalidArgument, "invalid page_token")
				})
			})

		})
	})
}

func toTestMetadataDetails(rows []*TestMetadataRow) (tmds []*pb.TestMetadataDetail) {
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
