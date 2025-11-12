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

package bqexporter

import (
	"context"
	"sort"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/bisection/model"
	bqpb "go.chromium.org/luci/bisection/proto/bq"
	bisectionpb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util/testutil"
)

func TestExport(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	testutil.UpdateIndices(ctx)
	cl := testclock.New(testclock.TestTimeUTC)
	baseTime := time.Unix(3600*24*30, 0).UTC()
	cl.Set(baseTime)
	ctx = clock.Set(ctx, cl)

	ftt.Run("export", t, func(t *ftt.Test) {
		// Create 5 test analyses.
		for i := 1; i <= 5; i++ {
			tfa := testutil.CreateTestFailureAnalysis(ctx, t, &testutil.TestFailureAnalysisCreationOption{
				ID:         int64(1000 + i),
				RunStatus:  bisectionpb.AnalysisRunStatus_ENDED,
				Status:     bisectionpb.AnalysisStatus(bisectionpb.AnalysisStatus_NOTFOUND),
				CreateTime: clock.Now(ctx).Add(time.Hour * time.Duration(-i)),
			})
			testutil.CreateTestFailure(ctx, t, &testutil.TestFailureCreationOption{
				ID:        int64(2000 + i),
				Analysis:  tfa,
				IsPrimary: true,
			})
		}

		client := &fakeExportClient{
			existingTestAnalyses: []*TestFailureAnalysisRow{
				{AnalysisID: 1001},
				{AnalysisID: 1003},
				{AnalysisID: 1005},
			},
		}
		err := export(ctx, client)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(client.rows), should.Equal(2))
		assert.Loosely(t, client.rows[0].AnalysisId, should.Equal(int64(1002)))
		assert.Loosely(t, client.rows[1].AnalysisId, should.Equal(int64(1004)))
	})
}

func TestExportCompile(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	testutil.UpdateIndices(ctx)
	cl := testclock.New(testclock.TestTimeUTC)
	baseTime := time.Unix(3600*24*30, 0).UTC()
	cl.Set(baseTime)
	ctx = clock.Set(ctx, cl)

	ftt.Run("export compile", t, func(t *ftt.Test) {
		// Create 5 compile analyses.
		for i := 1; i <= 5; i++ {
			_, _, cfa := testutil.CreateCompileFailureAnalysisAnalysisChain(ctx, t, int64(1000+i), "chromium", int64(1000+i))
			cfa.RunStatus = bisectionpb.AnalysisRunStatus_ENDED
			cfa.Status = bisectionpb.AnalysisStatus_NOTFOUND
			// Make sure the analyses are created more than 24 hours ago.
			cfa.CreateTime = clock.Now(ctx).Add(time.Hour * time.Duration(-24-i))
			assert.Loosely(t, datastore.Put(ctx, cfa), should.BeNil)
		}
		datastore.GetTestable(ctx).CatchupIndexes()

		client := &fakeExportClient{
			existingCompileAnalyses: []*CompileFailureAnalysisRow{
				{AnalysisID: 1001},
				{AnalysisID: 1003},
				{AnalysisID: 1005},
			},
		}
		err := exportCompile(ctx, client)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(client.compileRows), should.Equal(2))
		assert.Loosely(t, client.compileRows[0].AnalysisId, should.Equal(int64(1002)))
		assert.Loosely(t, client.compileRows[1].AnalysisId, should.Equal(int64(1004)))
	})
}

func TestFetchTestAnalyses(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	testutil.UpdateIndices(ctx)
	cl := testclock.New(testclock.TestTimeUTC)
	baseTime := time.Unix(3600*24*30, 0).UTC()
	cl.Set(baseTime)
	ctx = clock.Set(ctx, cl)

	ftt.Run("fetch test analyses", t, func(t *ftt.Test) {
		// Not ended, should be skipped.
		tf1 := testutil.CreateTestFailureAnalysis(ctx, t, &testutil.TestFailureAnalysisCreationOption{
			ID:         1001,
			RunStatus:  bisectionpb.AnalysisRunStatus_STARTED,
			CreateTime: clock.Now(ctx).Add(-time.Hour),
		})
		// Ended, but from a long time ago. Should be skipped.
		tf2 := testutil.CreateTestFailureAnalysis(ctx, t, &testutil.TestFailureAnalysisCreationOption{
			ID:         1002,
			RunStatus:  bisectionpb.AnalysisRunStatus_ENDED,
			CreateTime: clock.Now(ctx).Add(-15 * 24 * time.Hour),
		})
		// Ended, not found.
		tf3 := testutil.CreateTestFailureAnalysis(ctx, t, &testutil.TestFailureAnalysisCreationOption{
			ID:         1003,
			RunStatus:  bisectionpb.AnalysisRunStatus_ENDED,
			Status:     bisectionpb.AnalysisStatus(bisectionpb.AnalysisStatus_NOTFOUND),
			CreateTime: clock.Now(ctx).Add(-time.Hour),
		})
		// Ended, found, but action not taken, ended recently, should be skipped.
		tf4 := testutil.CreateTestFailureAnalysis(ctx, t, &testutil.TestFailureAnalysisCreationOption{
			ID:         1004,
			RunStatus:  bisectionpb.AnalysisRunStatus_ENDED,
			Status:     bisectionpb.AnalysisStatus(bisectionpb.AnalysisStatus_FOUND),
			CreateTime: clock.Now(ctx).Add(-time.Hour),
			EndTime:    clock.Now(ctx).Add(-time.Minute),
		})
		createSuspect(ctx, t, tf4, false)
		// Ended, found, actions taken.
		tf5 := testutil.CreateTestFailureAnalysis(ctx, t, &testutil.TestFailureAnalysisCreationOption{
			ID:         1005,
			RunStatus:  bisectionpb.AnalysisRunStatus_ENDED,
			Status:     bisectionpb.AnalysisStatus(bisectionpb.AnalysisStatus_FOUND),
			CreateTime: clock.Now(ctx).Add(-2 * time.Hour),
		})
		createSuspect(ctx, t, tf5, true)
		// Ended, found, but action not taken, ended long time ago.
		tf6 := testutil.CreateTestFailureAnalysis(ctx, t, &testutil.TestFailureAnalysisCreationOption{
			ID:         1006,
			RunStatus:  bisectionpb.AnalysisRunStatus_ENDED,
			Status:     bisectionpb.AnalysisStatus(bisectionpb.AnalysisStatus_FOUND),
			CreateTime: clock.Now(ctx).Add(-26 * time.Hour),
			EndTime:    clock.Now(ctx).Add(-25 * time.Hour),
		})
		createSuspect(ctx, t, tf6, false)
		assert.Loosely(t, datastore.Put(ctx, []*model.TestFailureAnalysis{tf1, tf2, tf3, tf4, tf5, tf6}), should.BeNil)
		datastore.GetTestable(ctx).CatchupIndexes()
		tfas, err := fetchTestAnalyses(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(tfas), should.Equal(3))
		assert.Loosely(t, tfas[0].ID, should.Equal(1003))
		assert.Loosely(t, tfas[1].ID, should.Equal(1005))
		assert.Loosely(t, tfas[2].ID, should.Equal(1006))
	})
}

func TestFetchCompileAnalyses(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	testutil.UpdateIndices(ctx)
	cl := testclock.New(testclock.TestTimeUTC)
	baseTime := time.Unix(3600*24*30, 0).UTC()
	cl.Set(baseTime)
	ctx = clock.Set(ctx, cl)

	ftt.Run("fetch compile analyses", t, func(t *ftt.Test) {
		// Not ended, should be skipped.
		_, _, cfa1 := testutil.CreateCompileFailureAnalysisAnalysisChain(ctx, t, 1001, "chromium", 1001)
		cfa1.RunStatus = bisectionpb.AnalysisRunStatus_STARTED
		cfa1.CreateTime = clock.Now(ctx).Add(-25 * time.Hour)

		// Ended, but from a long time ago. Should be skipped.
		_, _, cfa2 := testutil.CreateCompileFailureAnalysisAnalysisChain(ctx, t, 1002, "chromium", 1002)
		cfa2.RunStatus = bisectionpb.AnalysisRunStatus_ENDED
		cfa2.CreateTime = clock.Now(ctx).Add(-15 * 24 * time.Hour)

		// Ended, not found, but created recently. Should be skipped.
		_, _, cfa3 := testutil.CreateCompileFailureAnalysisAnalysisChain(ctx, t, 1003, "chromium", 1003)
		cfa3.RunStatus = bisectionpb.AnalysisRunStatus_ENDED
		cfa3.Status = bisectionpb.AnalysisStatus_NOTFOUND
		cfa3.CreateTime = clock.Now(ctx).Add(-time.Hour)

		// Ended, not found, created > 24 hours ago.
		_, _, cfa4 := testutil.CreateCompileFailureAnalysisAnalysisChain(ctx, t, 1004, "chromium", 1004)
		cfa4.RunStatus = bisectionpb.AnalysisRunStatus_ENDED
		cfa4.Status = bisectionpb.AnalysisStatus_NOTFOUND
		cfa4.CreateTime = clock.Now(ctx).Add(-25 * time.Hour)

		// Ended, found, actions taken, created recently. Should be skipped.
		_, _, cfa5 := testutil.CreateCompileFailureAnalysisAnalysisChain(ctx, t, 1005, "chromium", 1005)
		cfa5.RunStatus = bisectionpb.AnalysisRunStatus_ENDED
		cfa5.Status = bisectionpb.AnalysisStatus_FOUND
		cfa5.CreateTime = clock.Now(ctx).Add(-2 * time.Hour)
		createCompileSuspect(ctx, t, cfa5, true)

		// Ended, found, but action not taken, ended long time ago.
		_, _, cfa6 := testutil.CreateCompileFailureAnalysisAnalysisChain(ctx, t, 1006, "chromium", 1006)
		cfa6.RunStatus = bisectionpb.AnalysisRunStatus_ENDED
		cfa6.Status = bisectionpb.AnalysisStatus_FOUND
		cfa6.CreateTime = clock.Now(ctx).Add(-26 * time.Hour)
		cfa6.EndTime = clock.Now(ctx).Add(-25 * time.Hour)
		createCompileSuspect(ctx, t, cfa6, false)

		assert.Loosely(t, datastore.Put(ctx, []*model.CompileFailureAnalysis{cfa1, cfa2, cfa3, cfa4, cfa5, cfa6}), should.BeNil)
		datastore.GetTestable(ctx).CatchupIndexes()
		cfas, err := fetchCompileAnalyses(ctx)
		assert.Loosely(t, err, should.BeNil)

		// We check for the list of IDs, because the order is not guaranteed,
		// especially for items with the same creation time.
		actualIDs := make([]int64, len(cfas))
		for i, cfa := range cfas {
			actualIDs[i] = cfa.Id
		}
		expectedIDs := []int64{1004, 1006}
		sort.Slice(actualIDs, func(i, j int) bool { return actualIDs[i] < actualIDs[j] })
		sort.Slice(expectedIDs, func(i, j int) bool { return expectedIDs[i] < expectedIDs[j] })
		assert.Loosely(t, actualIDs, should.Resemble(expectedIDs))
	})
}

type HeuristicResult struct {
	Id int64 `gae:"$id"`
}

func createMixedCompileCulprits(ctx context.Context, t testing.TB, cfa *model.CompileFailureAnalysis, hasTakenAction bool) {
	suspect := &model.Suspect{
		Id: cfa.Id,
		ActionDetails: model.ActionDetails{
			HasTakenActions: hasTakenAction,
		},
	}
	assert.Loosely(t, datastore.Put(ctx, suspect), should.BeNil, truth.LineContext())
	heuristic := &HeuristicResult{
		Id: cfa.Id + 1000,
	}
	assert.Loosely(t, datastore.Put(ctx, heuristic), should.BeNil, truth.LineContext())
	datastore.GetTestable(ctx).CatchupIndexes()
	cfa.VerifiedCulprits = []*datastore.Key{
		datastore.KeyForObj(ctx, suspect),
		datastore.KeyForObj(ctx, heuristic),
	}
}

func createSuspect(ctx context.Context, t testing.TB, tfa *model.TestFailureAnalysis, hasTakenAction bool) {
	suspect := &model.Suspect{
		Id: tfa.ID,
		ActionDetails: model.ActionDetails{
			HasTakenActions: hasTakenAction,
		},
	}
	assert.Loosely(t, datastore.Put(ctx, suspect), should.BeNil, truth.LineContext())
	datastore.GetTestable(ctx).CatchupIndexes()
	tfa.VerifiedCulpritKey = datastore.KeyForObj(ctx, suspect)
}

func createCompileSuspect(ctx context.Context, t testing.TB, cfa *model.CompileFailureAnalysis, hasTakenAction bool) {
	suspect := &model.Suspect{
		Id: cfa.Id,
		ActionDetails: model.ActionDetails{
			HasTakenActions: hasTakenAction,
		},
	}
	assert.Loosely(t, datastore.Put(ctx, suspect), should.BeNil, truth.LineContext())
	datastore.GetTestable(ctx).CatchupIndexes()
	cfa.VerifiedCulprits = []*datastore.Key{datastore.KeyForObj(ctx, suspect)}
}

// Fake client.
type fakeExportClient struct {
	rows                    []*bqpb.TestAnalysisRow
	compileRows             []*bqpb.CompileAnalysisRow
	existingTestAnalyses    []*TestFailureAnalysisRow
	existingCompileAnalyses []*CompileFailureAnalysisRow
}

func (cl *fakeExportClient) EnsureSchema(ctx context.Context) error {
	return nil
}

func (cl *fakeExportClient) Insert(ctx context.Context, rows []*bqpb.TestAnalysisRow) error {
	cl.rows = append(cl.rows, rows...)
	return nil
}

func (cl *fakeExportClient) ReadTestFailureAnalysisRows(ctx context.Context) ([]*TestFailureAnalysisRow, error) {
	if cl.existingTestAnalyses != nil {
		return cl.existingTestAnalyses, nil
	}
	return []*TestFailureAnalysisRow{}, nil
}

func (cl *fakeExportClient) EnsureCompileAnalysisSchema(ctx context.Context) error {
	return nil
}

func (cl *fakeExportClient) InsertCompileAnalysisRows(ctx context.Context, rows []*bqpb.CompileAnalysisRow) error {
	cl.compileRows = append(cl.compileRows, rows...)
	return nil
}

func (cl *fakeExportClient) ReadCompileFailureAnalysisRows(ctx context.Context) ([]*CompileFailureAnalysisRow, error) {
	if cl.existingCompileAnalyses != nil {
		return cl.existingCompileAnalyses, nil
	}
	return []*CompileFailureAnalysisRow{}, nil
}
