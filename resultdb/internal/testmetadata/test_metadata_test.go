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
	"sort"
	"testing"
	"time"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestReadTestMetadata(t *testing.T) {
	ftt.Run(`ReadTestMetadata`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		sourceRef := &pb.SourceRef{
			System: &pb.SourceRef_Gitiles{
				Gitiles: &pb.GitilesRef{
					Host:    "testhost",
					Project: "testproject",
					Ref:     "testref",
				},
			}}
		hash := pbutil.SourceRefHash(sourceRef)
		run := func(opts ReadTestMetadataOptions) (rows []*TestMetadataRow, err error) {
			ctx, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()
			rows = make([]*TestMetadataRow, 0)
			err = ReadTestMetadata(ctx, opts, func(tmd *TestMetadataRow) error {
				rows = append(rows, tmd)
				return nil
			})
			return rows, err
		}
		verify := func(actual, expected []*TestMetadataRow) {
			assert.Loosely(t, actual, should.HaveLength(len(expected)))
			sortMetadata(actual)
			sortMetadata(expected)

			for i, row := range actual {
				expectedRow := *expected[i]
				// ShouldResemble does not work on struct with nested proto buffer.
				// So we compare each proto field separately.
				assert.Loosely(t, row.TestMetadata, should.Match(expectedRow.TestMetadata))
				assert.Loosely(t, row.SourceRef, should.Match(expectedRow.SourceRef))
				cloneActual := *row
				cloneActual.TestMetadata = nil
				cloneActual.SourceRef = nil
				expectedRow.TestMetadata = nil
				expectedRow.SourceRef = nil
				cloneActual.LastUpdated = time.Time{}
				expectedRow.LastUpdated = time.Time{}
				assert.Loosely(t, cloneActual, should.Match(expectedRow))
			}
		}
		t.Run(`Does not fetch test metadata from other test, source ref or realm`, func(t *ftt.Test) {
			tmr1 := makeTestMetadataRow("testproject", "test1", "testrealm", hash)
			tmr2 := makeTestMetadataRow("testproject", "test2", "testrealm", hash)
			tmr3 := makeTestMetadataRow("testproject", "test3", "testrealm", hash)
			tmr4 := makeTestMetadataRow("testproject", "test1", "othertestrealm", hash)
			tmr5 := makeTestMetadataRow("testproject", "test1", "testrealm", []byte("hash2"))
			tmr6 := makeTestMetadataRow("otherproject", "test1", "testrealm", hash)
			testutil.MustApply(ctx, t, insertTestMetadataRows([]*TestMetadataRow{tmr1, tmr2, tmr3, tmr4, tmr5, tmr6})...)
			opts := ReadTestMetadataOptions{
				Project:   "testproject",
				TestIDs:   []string{"test1", "test2"},
				SourceRef: sourceRef,
				SubRealm:  "testrealm",
			}

			actual, err := run(opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.HaveLength(2))
			verify(actual, []*TestMetadataRow{tmr1, tmr2})
		})
	})
}

func sortMetadata(testMetadataRows []*TestMetadataRow) {
	combineKey := func(row *TestMetadataRow) string {
		return row.Project + "\n" + row.TestID + "\n" + hex.EncodeToString(row.RefHash) + "\n" + row.SubRealm
	}
	sort.Slice(testMetadataRows, func(i, j int) bool {
		return combineKey(testMetadataRows[i]) < combineKey(testMetadataRows[j])
	})
}

func insertTestMetadataRows(rows []*TestMetadataRow) []*spanner.Mutation {
	ms := make([]*spanner.Mutation, len(rows))
	for i, row := range rows {
		mutMap := map[string]any{
			"Project":      row.Project,
			"TestId":       row.TestID,
			"SubRealm":     row.SubRealm,
			"RefHash":      row.RefHash,
			"LastUpdated":  row.LastUpdated,
			"TestMetadata": spanutil.Compressed(pbutil.MustMarshal(row.TestMetadata)),
			"SourceRef":    spanutil.Compressed(pbutil.MustMarshal(row.SourceRef)),
			"Position":     int64(row.Position),
		}
		ms[i] = spanutil.InsertMap("TestMetadata", mutMap)
	}
	return ms
}

func makeTestMetadataRow(project, testID, subRealm string, refHash []byte) *TestMetadataRow {
	return &TestMetadataRow{
		Project:     project,
		TestID:      testID,
		RefHash:     refHash,
		SubRealm:    subRealm,
		LastUpdated: time.Time{},
		TestMetadata: &pb.TestMetadata{
			Name: project + "\n" + testID + "\n" + subRealm,
			Location: &pb.TestLocation{
				Repo:     "testRepo",
				FileName: "testFile",
				Line:     0,
			},
			BugComponent: &pb.BugComponent{},
		},
		SourceRef: &pb.SourceRef{
			System: &pb.SourceRef_Gitiles{
				Gitiles: &pb.GitilesRef{Host: "testHost"},
			},
		},
		Position: 0,
	}
}
