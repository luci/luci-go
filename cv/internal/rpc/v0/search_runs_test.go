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

package rpc

import (
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	apiv0pb "go.chromium.org/luci/cv/api/v0"
	"go.chromium.org/luci/cv/internal/acls"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"

	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestSearchRuns(t *testing.T) {
	t.Parallel()

	ftt.Run("SearchRuns", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		srv := RunsServer{}

		const projectName = "prj"

		prjcfgtest.Create(ctx, projectName, &cfgpb.Config{
			// TODO(crbug/1233963): remove once non-legacy ACLs are implemented.
			CqStatusHost: "chromium-cq-status.appspot.com",
			ConfigGroups: []*cfgpb.ConfigGroup{{Name: "first"}},
		})

		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity:       "user:admin@example.com",
			IdentityGroups: []string{acls.V0APIAllowGroup},
		})

		t.Run("without access", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := srv.SearchRuns(ctx, &apiv0pb.SearchRunsRequest{
				Predicate: &apiv0pb.RunPredicate{Project: projectName},
			})
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)())
		})

		t.Run("with no predicate", func(t *ftt.Test) {
			_, err := srv.SearchRuns(ctx, &apiv0pb.SearchRunsRequest{})
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)())

			_, err = srv.SearchRuns(ctx, &apiv0pb.SearchRunsRequest{PageSize: 50})
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)())
		})

		t.Run("with a page size that is too large", func(t *ftt.Test) {
			_, err := srv.SearchRuns(ctx, &apiv0pb.SearchRunsRequest{PageSize: maxPageSize + 1})
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)())
		})

		t.Run("with no project", func(t *ftt.Test) {
			_, err := srv.SearchRuns(ctx, &apiv0pb.SearchRunsRequest{
				Predicate: &apiv0pb.RunPredicate{},
			})
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)())
		})

		t.Run("with nonexistent project", func(t *ftt.Test) {
			resp, err := srv.SearchRuns(ctx, &apiv0pb.SearchRunsRequest{
				Predicate: &apiv0pb.RunPredicate{Project: "bogus"},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp.Runs, should.BeEmpty)
			assert.Loosely(t, resp.NextPageToken, should.BeEmpty)
		})

		t.Run("with no runs", func(t *ftt.Test) {
			resp, err := srv.SearchRuns(ctx, &apiv0pb.SearchRunsRequest{
				Predicate: &apiv0pb.RunPredicate{Project: projectName},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp.Runs, should.BeEmpty)
			assert.Loosely(t, resp.NextPageToken, should.BeEmpty)
		})

		// Add example data for tests below.
		gHost := "r-review.example.com"
		epoch := testclock.TestRecentTimeUTC.Truncate(time.Millisecond)

		// putRun puts a Run and RunCLs in datastore and returns the Run.
		putRun := func(proj string, delay time.Duration, cls ...*changelist.CL) *run.Run {
			createdAt := epoch.Add(delay)
			// As long as each RunID used in a given test is unique, RunID
			// details don't matter. So practically, each Run must have a
			// different create time or different set of CLs.
			var clsDigest []byte
			for _, cl := range cls {
				clsDigest = append(clsDigest, []byte(cl.ExternalID)...)
			}
			runID := common.MakeRunID(proj, createdAt, 1, clsDigest)
			clids := make(common.CLIDs, len(cls))
			for i, cl := range cls {
				clids[i] = common.CLID(cl.ID)
			}
			r := &run.Run{
				ID:         runID,
				Status:     run.Status_SUCCEEDED,
				CLs:        clids,
				CreateTime: createdAt,
				StartTime:  createdAt.Add(time.Second),
				UpdateTime: createdAt.Add(time.Minute),
				EndTime:    createdAt.Add(time.Hour),
				Owner:      "user:foo@example.org",
			}
			assert.Loosely(t, datastore.Put(ctx, r), should.BeNil)
			for _, cl := range cls {
				assert.Loosely(t, datastore.Put(ctx, &run.RunCL{
					Run:        datastore.MakeKey(ctx, common.RunKind, string(runID)),
					ID:         cl.ID,
					IndexedID:  cl.ID,
					ExternalID: cl.ExternalID,
					Detail:     &changelist.Snapshot{Patchset: 1},
				}), should.BeNil)
			}
			return r
		}

		t.Run("with matching Runs, project-only predicate", func(t *ftt.Test) {
			cl1 := changelist.MustGobID(gHost, 1).MustCreateIfNotExists(ctx)
			cl2 := changelist.MustGobID(gHost, 2).MustCreateIfNotExists(ctx)
			r1 := putRun(projectName, 1*time.Millisecond, cl1)
			r2 := putRun(projectName, 5*time.Millisecond, cl2)
			resp, err := srv.SearchRuns(ctx, &apiv0pb.SearchRunsRequest{
				Predicate: &apiv0pb.RunPredicate{Project: projectName},
			})
			assert.Loosely(t, err, should.BeNil)

			// Most recent Run comes first.
			assert.Loosely(t, respIDs(resp.Runs), should.Resemble(runIDs(r2, r1)))
			assert.Loosely(t, resp.NextPageToken, should.BeEmpty)
		})

		t.Run("paging, project-only predicate", func(t *ftt.Test) {
			cl1 := changelist.MustGobID(gHost, 1).MustCreateIfNotExists(ctx)
			cl2 := changelist.MustGobID(gHost, 2).MustCreateIfNotExists(ctx)
			r1 := putRun(projectName, 1*time.Millisecond, cl1)
			r2 := putRun(projectName, 5*time.Millisecond, cl2)

			// First request, first page.
			resp, err := srv.SearchRuns(ctx, &apiv0pb.SearchRunsRequest{
				Predicate: &apiv0pb.RunPredicate{Project: projectName},
				PageSize:  1,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, respIDs(resp.Runs), should.Resemble(runIDs(r2)))
			assert.Loosely(t, resp.NextPageToken, should.NotBeEmpty)

			// Second request, second page.
			resp, err = srv.SearchRuns(ctx, &apiv0pb.SearchRunsRequest{
				Predicate: &apiv0pb.RunPredicate{Project: projectName},
				PageSize:  1,
				PageToken: resp.NextPageToken,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, respIDs(resp.Runs), should.Resemble(runIDs(r1)))
			assert.Loosely(t, resp.NextPageToken, should.NotBeEmpty)

			// Third request, no more results.
			resp, err = srv.SearchRuns(ctx, &apiv0pb.SearchRunsRequest{
				Predicate: &apiv0pb.RunPredicate{Project: projectName},
				PageSize:  1,
				PageToken: resp.NextPageToken,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp.Runs, should.BeEmpty)
			assert.Loosely(t, resp.NextPageToken, should.BeEmpty)
		})

		t.Run("with matching Run, single CL predicate", func(t *ftt.Test) {
			cl1 := changelist.MustGobID(gHost, 1).MustCreateIfNotExists(ctx)
			r1 := putRun(projectName, 1*time.Millisecond, cl1)
			resp, err := srv.SearchRuns(ctx, &apiv0pb.SearchRunsRequest{
				Predicate: &apiv0pb.RunPredicate{
					Project: projectName,
					GerritChanges: []*apiv0pb.GerritChange{
						{Host: gHost, Change: 1},
					},
				},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, respIDs(resp.Runs), should.Resemble(runIDs(r1)))
		})

		t.Run("with CL predicate that includes patchset", func(t *ftt.Test) {
			cl1 := changelist.MustGobID(gHost, 1).MustCreateIfNotExists(ctx)
			putRun(projectName, 1*time.Millisecond, cl1)
			_, err := srv.SearchRuns(ctx, &apiv0pb.SearchRunsRequest{
				Predicate: &apiv0pb.RunPredicate{
					Project: projectName,
					GerritChanges: []*apiv0pb.GerritChange{
						{Host: gHost, Change: 1, Patchset: 3},
					},
				},
			})
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)())
		})

		t.Run("with CL predicate and no project given", func(t *ftt.Test) {
			cl1 := changelist.MustGobID(gHost, 1).MustCreateIfNotExists(ctx)
			putRun(projectName, 1*time.Millisecond, cl1)
			_, err := srv.SearchRuns(ctx, &apiv0pb.SearchRunsRequest{
				Predicate: &apiv0pb.RunPredicate{
					GerritChanges: []*apiv0pb.GerritChange{
						{Host: gHost, Change: 1},
					},
				},
			})
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)())
		})

		t.Run("with no matching Run, CL predicate", func(t *ftt.Test) {
			// No Runs put.
			resp, err := srv.SearchRuns(ctx, &apiv0pb.SearchRunsRequest{
				Predicate: &apiv0pb.RunPredicate{
					Project: projectName,
					GerritChanges: []*apiv0pb.GerritChange{
						{Host: gHost, Change: 1},
					},
				},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp.Runs, should.BeEmpty)
		})

		t.Run("query with multiple CLs returns Run that contains all CLs", func(t *ftt.Test) {
			cl1 := changelist.MustGobID(gHost, 2).MustCreateIfNotExists(ctx)
			cl2 := changelist.MustGobID(gHost, 3).MustCreateIfNotExists(ctx)
			r1 := putRun(projectName, 1*time.Millisecond, cl1, cl2)
			resp, err := srv.SearchRuns(ctx, &apiv0pb.SearchRunsRequest{
				Predicate: &apiv0pb.RunPredicate{
					Project: projectName,
					GerritChanges: []*apiv0pb.GerritChange{
						{
							Host:   gHost,
							Change: 2,
						},
						{
							Host:   gHost,
							Change: 3,
						},
					},
				},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, respIDs(resp.Runs), should.Resemble(runIDs(r1)))
		})

		t.Run("query with multiple CLs returns nothing if no single CL contains all CLs", func(t *ftt.Test) {
			cl1 := changelist.MustGobID(gHost, 1).MustCreateIfNotExists(ctx)
			cl2 := changelist.MustGobID(gHost, 2).MustCreateIfNotExists(ctx)
			putRun(projectName, 1*time.Millisecond, cl1)
			putRun(projectName, 5*time.Millisecond, cl2)
			resp, err := srv.SearchRuns(ctx, &apiv0pb.SearchRunsRequest{
				Predicate: &apiv0pb.RunPredicate{
					Project: projectName,
					GerritChanges: []*apiv0pb.GerritChange{
						{
							Host:   gHost,
							Change: 1,
						},
						{
							Host:   gHost,
							Change: 2,
						},
					},
				},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp.Runs, should.BeEmpty)
		})
	})
}

func respIDs(runs []*apiv0pb.Run) common.RunIDs {
	var ret common.RunIDs
	for _, r := range runs {
		id, err := common.FromPublicRunID(r.Id)
		if err != nil {
			panic(err)
		}
		ret = append(ret, id)
	}
	return ret
}

func runIDs(runs ...*run.Run) common.RunIDs {
	var ret common.RunIDs
	for _, r := range runs {
		ret = append(ret, r.ID)
	}
	return ret
}
