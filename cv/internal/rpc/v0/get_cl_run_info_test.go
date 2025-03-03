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
	"fmt"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/gerritauth"

	apiv0pb "go.chromium.org/luci/cv/api/v0"
	"go.chromium.org/luci/cv/internal/acls"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
)

func TestGetCLRunInfo(t *testing.T) {
	t.Parallel()

	ftt.Run("GetCLRunInfo", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		gis := GerritIntegrationServer{}

		const host = "chromium"
		const hostReview = host + "-review.googlesource.com"
		gc := &apiv0pb.GerritChange{
			Host:     hostReview,
			Change:   1,
			Patchset: 39,
		}

		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity:       "user:admin@example.com",
			IdentityGroups: []string{acls.V0APIAllowGroup, common.InstantTriggerDogfooderGroup},
			UserExtra: &gerritauth.AssertedInfo{
				Change: gerritauth.AssertedChange{
					Host:         host,
					ChangeNumber: 1,
				},
				User: gerritauth.AssertedUser{
					AccountID:      12345,
					PreferredEmail: "admin@example.com",
				},
			},
		})

		t.Run("w/o access", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := gis.GetCLRunInfo(ctx, &apiv0pb.GetCLRunInfoRequest{GerritChange: gc})
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.PermissionDenied))
		})

		t.Run("w/o access but with JWT", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
				UserExtra: &gerritauth.AssertedInfo{
					Change: gerritauth.AssertedChange{
						Host:         host,
						ChangeNumber: 1,
					},
					User: gerritauth.AssertedUser{
						AccountID:      12345,
						PreferredEmail: "admin@example.com",
					},
				},
			})
			ct.ResetMockedAuthDB(ctx)
			ct.AddMember("admin@example.com", common.InstantTriggerDogfooderGroup)
			_, err := gis.GetCLRunInfo(ctx, &apiv0pb.GetCLRunInfoRequest{GerritChange: gc})
			// NotFound because we haven't put anything in the datastore yet.
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.NotFound))
		})

		t.Run("w/ access but no JWT", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{acls.V0APIAllowGroup},
			})
			_, err := gis.GetCLRunInfo(ctx, &apiv0pb.GetCLRunInfoRequest{GerritChange: gc})
			// NotFound because we haven't put anything in the datastore yet.
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.NotFound))
		})

		t.Run("w/ an invalid Gerrit Change", func(t *ftt.Test) {
			invalidGc := &apiv0pb.GerritChange{
				Host:     "bad/host.example.com",
				Change:   1,
				Patchset: 39,
			}
			_, err := gis.GetCLRunInfo(ctx, &apiv0pb.GetCLRunInfoRequest{GerritChange: invalidGc})
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.InvalidArgument))
		})

		t.Run("w/ a Valid but missing Gerrit Change", func(t *ftt.Test) {
			_, err := gis.GetCLRunInfo(ctx, &apiv0pb.GetCLRunInfoRequest{GerritChange: gc})
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.NotFound))
		})

		t.Run("w/ JWT change differing from Gerrit Change", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{acls.V0APIAllowGroup},
				UserExtra: &gerritauth.AssertedInfo{
					Change: gerritauth.AssertedChange{
						Host:         "other-host",
						ChangeNumber: 1,
					},
				},
			})
			_, err := gis.GetCLRunInfo(ctx, &apiv0pb.GetCLRunInfoRequest{GerritChange: gc})
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.InvalidArgument))
		})

		// Add example data for tests below.

		const owner = "owner@example.com"

		// addRunAndGetRunInfo populates the datastore with an example Run associated with the change
		// and returns the expected RunInfo.
		addRunAndGetRunInfo := func(c *apiv0pb.GerritChange) *apiv0pb.GetCLRunInfoResponse_RunInfo {
			cl := changelist.MustGobID(c.Host, c.Change).MustCreateIfNotExists(ctx)
			epoch := testclock.TestRecentTimeUTC.Truncate(time.Millisecond)
			rid := common.RunID("prj/123-deadbeef")
			r := &run.Run{
				ID:         rid,
				Status:     run.Status_RUNNING,
				CreateTime: epoch,
				StartTime:  epoch.Add(time.Second),
				UpdateTime: epoch.Add(time.Minute),
				EndTime:    epoch.Add(time.Hour),
				Owner:      "user:foo@example.org",
				CLs:        common.MakeCLIDs(int64(cl.ID)),
				Mode:       run.FullRun,
				RootCL:     cl.ID,
			}
			rcl := &run.RunCL{
				Run: datastore.MakeKey(ctx, common.RunKind, string(r.ID)),
				ID:  cl.ID, IndexedID: cl.ID,
				ExternalID: cl.ExternalID,
				Detail: &changelist.Snapshot{
					Patchset: 39,
				},
			}
			assert.Loosely(t, datastore.Put(ctx, rcl), should.BeNil)
			assert.Loosely(t, datastore.Put(ctx, r), should.BeNil)
			return &apiv0pb.GetCLRunInfoResponse_RunInfo{
				Id:           fmt.Sprintf("projects/%s/runs/%s", rid.LUCIProject(), rid.Inner()),
				CreateTime:   timestamppb.New(r.CreateTime),
				StartTime:    timestamppb.New(r.StartTime),
				Mode:         string(r.Mode),
				OriginChange: c,
			}
		}

		// setSnapshot sets a CL's snapshot.
		setSnapshot := func(cl *changelist.CL, gc *apiv0pb.GerritChange, deps []*changelist.Dep) {
			cl.Snapshot = &changelist.Snapshot{
				Kind: &changelist.Snapshot_Gerrit{
					Gerrit: &changelist.Gerrit{
						Host: gc.Host,
						Info: &gerritpb.ChangeInfo{
							Owner: &gerritpb.AccountInfo{
								Email: owner,
							},
							Number: gc.Change,
							Status: gerritpb.ChangeStatus_NEW,
						},
					},
				},
				Patchset: gc.Patchset,
				Deps:     deps,
			}
		}

		// putWithDeps stores a GerritChange and its dependencies in datastore.
		putWithDeps := func(change *apiv0pb.GerritChange, depChanges []*apiv0pb.GerritChange) {
			// Add deps.
			deps := make([]*changelist.Dep, len(depChanges))
			for i, dc := range depChanges {
				eid := changelist.MustGobID(dc.Host, dc.Change)
				depCl := eid.MustCreateIfNotExists(ctx)
				setSnapshot(depCl, dc, nil)
				assert.Loosely(t, datastore.Put(ctx, depCl), should.BeNil)
				deps[i] = &changelist.Dep{
					Clid: int64(depCl.ID),
				}
			}

			// Add the CL itself.
			eid := changelist.MustGobID(change.Host, change.Change)
			cl := eid.MustCreateIfNotExists(ctx)
			setSnapshot(cl, change, deps)
			assert.Loosely(t, datastore.Put(ctx, cl), should.BeNil)
		}

		t.Run("DepChangeInfos w/ valid Gerrit Change and no deps", func(t *ftt.Test) {
			putWithDeps(gc, nil)

			resp, err := gis.GetCLRunInfo(ctx, &apiv0pb.GetCLRunInfoRequest{GerritChange: gc})
			assert.NoErr(t, err)
			assert.Loosely(t, resp.DepChangeInfos, should.BeEmpty)
		})

		t.Run("DepChangeInfos w/ valid Gerrit Change and deps", func(t *ftt.Test) {
			deps := []*apiv0pb.GerritChange{
				{
					Host:     hostReview,
					Change:   2,
					Patchset: 1,
				},
				{
					Host:     hostReview,
					Change:   3,
					Patchset: 1,
				},
			}
			putWithDeps(gc, deps)
			// Add an ongoing run to the first dep.
			runInfo := addRunAndGetRunInfo(deps[0])

			resp, err := gis.GetCLRunInfo(ctx, &apiv0pb.GetCLRunInfoRequest{GerritChange: gc})
			assert.NoErr(t, err)
			assert.That(t, resp.DepChangeInfos, should.Match([]*apiv0pb.GetCLRunInfoResponse_DepChangeInfo{
				{
					GerritChange: deps[0],
					ChangeOwner:  owner,
					Runs:         []*apiv0pb.GetCLRunInfoResponse_RunInfo{runInfo},
				},
				{
					GerritChange: deps[1],
					ChangeOwner:  owner,
					Runs:         []*apiv0pb.GetCLRunInfoResponse_RunInfo{},
				},
			}))

			t.Run("skip submitted dep", func(t *ftt.Test) {
				cl, err := changelist.MustGobID(deps[0].GetHost(), deps[0].GetChange()).Load(ctx)
				assert.NoErr(t, err)
				cl.Snapshot.GetGerrit().GetInfo().Status = gerritpb.ChangeStatus_MERGED
				assert.Loosely(t, datastore.Put(ctx, cl), should.BeNil)
				resp, err := gis.GetCLRunInfo(ctx, &apiv0pb.GetCLRunInfoRequest{GerritChange: gc})
				assert.NoErr(t, err)
				assert.That(t, resp.DepChangeInfos, should.Match([]*apiv0pb.GetCLRunInfoResponse_DepChangeInfo{
					{
						GerritChange: deps[1],
						ChangeOwner:  owner,
						Runs:         []*apiv0pb.GetCLRunInfoResponse_RunInfo{},
					},
				}))
			})
			t.Run("return empty response for non-dogfooder", func(t *ftt.Test) {
				ct.ResetMockedAuthDB(ctx)
				resp, err := gis.GetCLRunInfo(ctx, &apiv0pb.GetCLRunInfoRequest{GerritChange: gc})
				assert.NoErr(t, err)
				assert.Loosely(t, resp.GetRunsAsOrigin(), should.BeEmpty)
				assert.Loosely(t, resp.GetRunsAsDep(), should.BeEmpty)
				assert.Loosely(t, resp.GetDepChangeInfos(), should.BeEmpty)
			})
			t.Run("return empty response if user email is missing in jwt", func(t *ftt.Test) {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity:       "user:admin@example.com",
					IdentityGroups: []string{acls.V0APIAllowGroup, common.InstantTriggerDogfooderGroup},
					UserExtra: &gerritauth.AssertedInfo{
						Change: gerritauth.AssertedChange{
							Host:         host,
							ChangeNumber: 1,
						},
					},
				})
				resp, err := gis.GetCLRunInfo(ctx, &apiv0pb.GetCLRunInfoRequest{GerritChange: gc})
				assert.NoErr(t, err)
				assert.Loosely(t, resp.GetRunsAsOrigin(), should.BeEmpty)
				assert.Loosely(t, resp.GetRunsAsDep(), should.BeEmpty)
				assert.Loosely(t, resp.GetDepChangeInfos(), should.BeEmpty)
			})
		})
	})
}
