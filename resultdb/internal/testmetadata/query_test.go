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
	"sort"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/span"
)

func TestQueryTestMetadata(t *testing.T) {

	sort := func(testMetadataRows []*TestMetadataRow) {
		combineKey := func(row *TestMetadataRow) string {
			return row.Project + "\n" + row.TestID + "\n" + string(row.RefHash) + "\n" + row.SubRealm
		}
		sort.Slice(testMetadataRows, func(i, j int) bool {
			return combineKey(testMetadataRows[i]) < combineKey(testMetadataRows[j])
		})
	}
	verify := func(actual, expected []*TestMetadataRow) {
		So(actual, ShouldHaveLength, len(expected))
		sort(actual)
		sort(expected)
		lastUpdated := time.Now()
		for i, row := range actual {
			expectedRow := expected[i]
			// ShouldResemble does not work on struct with nested proto buffer.
			// So we compare each proto field separately.
			So(row.TestMetadata, ShouldResembleProto, expectedRow.TestMetadata)
			So(row.SourceRef, ShouldResembleProto, expectedRow.SourceRef)
			row.TestMetadata = nil
			row.SourceRef = nil
			expectedRow.TestMetadata = nil
			expectedRow.SourceRef = nil
			So(withLastUpdated(row, lastUpdated), ShouldResemble, withLastUpdated(expectedRow, lastUpdated))
		}
	}
	Convey(`QueryTestMetadata`, t, func() {

		ctx := testutil.SpannerTestContext(t)
		sourceRef := &pb.SourceRef{
			System: &pb.SourceRef_Gitiles{
				Gitiles: &pb.GitilesRef{
					Host:    "testhost",
					Project: "testproject",
					Ref:     "testref",
				},
			}}
		q := &Query{
			Project:   "testproject",
			TestIDs:   []string{"test1", "test2"},
			SourceRef: sourceRef,
			SubRealm:  "testrealm",
		}
		hash := pbutil.RefHash(sourceRef)

		fetch := func(q *Query) (rows []*TestMetadataRow, err error) {
			ctx, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()
			rows = make([]*TestMetadataRow, 0)
			err = q.Run(ctx, func(tmd *TestMetadataRow) error {
				rows = append(rows, tmd)
				return nil
			})
			return rows, err
		}

		Convey(`Does not fetch test metadata from other test, source ref or realm`, func() {
			committime := testutil.MustApply(ctx,
				insert.TestMetadata("testproject", "test1", "testrealm", hash, map[string]any{"Position": 1}),
				insert.TestMetadata("testproject", "test2", "testrealm", hash, map[string]any{"Position": 1}),
				insert.TestMetadata("testproject", "test3", "testrealm", hash, map[string]any{"Position": 1}),
				insert.TestMetadata("testproject", "test1", "othertestrealm", hash, map[string]any{"Position": 1}),
				insert.TestMetadata("testproject", "test1", "testrealm", []byte("hash2"), map[string]any{"Position": 1}),
				insert.TestMetadata("otherproject", "test1", "testrealm", hash, map[string]any{"Position": 1}),
			)

			actual, err := fetch(q)
			So(err, ShouldBeNil)
			So(actual, ShouldHaveLength, 2)
			verify(actual, []*TestMetadataRow{
				{
					Project:      "testproject",
					TestID:       "test1",
					RefHash:      hash,
					SubRealm:     "testrealm",
					LastUpdated:  committime,
					TestMetadata: &pb.TestMetadata{},
					Position:     1,
				},
				{
					Project:      "testproject",
					TestID:       "test2",
					RefHash:      hash,
					SubRealm:     "testrealm",
					LastUpdated:  committime,
					TestMetadata: &pb.TestMetadata{},
					Position:     1,
				},
			})

		})
	})
}

func withLastUpdated(testMetadataRow *TestMetadataRow, lastupdated time.Time) *TestMetadataRow {
	testMetadataRow.LastUpdated = lastupdated
	return testMetadataRow
}
