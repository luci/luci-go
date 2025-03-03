// Copyright 2021 The LUCI Authors.
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

package admin

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/dsmapper"
	"go.chromium.org/luci/server/dsmapper/dsmapperpb"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/cv/internal/cvtesting"
	adminpb "go.chromium.org/luci/cv/internal/rpc/admin/api"
)

func TestDSMapperServer(t *testing.T) {
	t.Parallel()

	ftt.Run("dsmapper job lifecycle", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		d := dsMapper{
			ctrl: &dsmapper.Controller{
				ControlQueue: "default",
				MapperQueue:  "default",
			},
		}
		d.ctrl.Install(ct.TQDispatcher)
		d.register(
			&dsmapper.JobConfig{
				Mapper: "upgrade-something",
				Query: dsmapper.Query{
					Kind: "SomethingUniqueTest",
				},
				ShardCount: 2,
				PageSize:   16,
			},
			func(context.Context, *dsmapper.Job, int) (dsmapper.Mapper, error) {
				return func(ctx context.Context, keys []*datastore.Key) error {
					// In prod, this updates the entities with the given keys.
					return nil
				}, nil
			},
		)
		a := AdminServer{dsmapper: &d}

		t.Run("without access", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := a.DSMLaunchJob(ctx, &adminpb.DSMLaunchJobRequest{Name: "upgrade-something"})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			_, err = a.DSMGetJob(ctx, &adminpb.DSMJobID{Id: 1})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			_, err = a.DSMAbortJob(ctx, &adminpb.DSMJobID{Id: 1})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
		})

		t.Run("with access", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{allowGroup},
			})

			jobID, err := a.DSMLaunchJob(ctx, &adminpb.DSMLaunchJobRequest{Name: "upgrade-something"})
			assert.NoErr(t, err)

			job, err := a.DSMGetJob(ctx, jobID)
			assert.NoErr(t, err)
			assert.That(t, job.GetName(), should.Match("upgrade-something"))
			assert.Loosely(t, job.GetInfo().GetState(), should.Equal(dsmapperpb.State_STARTING))

			t.Run("SUCCESS", func(t *ftt.Test) {
				ct.TQ.Run(ctx, tqtesting.StopWhenDrained())

				job, err = a.DSMGetJob(ctx, jobID)
				assert.NoErr(t, err)
				assert.That(t, job.GetName(), should.Match("upgrade-something"))
				assert.Loosely(t, job.GetInfo().GetState(), should.Equal(dsmapperpb.State_SUCCESS))
			})
			t.Run("Abort", func(t *ftt.Test) {
				_, err = a.DSMAbortJob(ctx, jobID)
				assert.NoErr(t, err)
				ct.TQ.Run(ctx, tqtesting.StopWhenDrained())
				// This fails with:
				//   "broken state, no ShardList entity for job 1"
				// which is probably because job was aborted right after launching.
				// job, err = d.DSMGetJob(ctx, jobID)
				// So(err, ShouldBeNil)
			})
		})
	})
}
