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

// Package cancel implements tasks to do when canceling a build.
//
// The functions will need to be used from multiple places in the buildbucket flow:
// * in CancelBuild,
// * in CancelBuildTask,
// * in UpdateBuild, if the build's parent has terminated and the build cannot
//   outlive its parent.
package cancel

import (
	"context"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/buildbucket/appengine/internal/notify"
	"go.chromium.org/luci/buildbucket/appengine/internal/perm"
	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/appengine/tasks"
	taskdefs "go.chromium.org/luci/buildbucket/appengine/tasks/defs"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

// Do cancels a build.
func Do(ctx context.Context, bID int64, summary string) (*model.Build, error) {
	bld := &model.Build{ID: bID}
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		inf := &model.BuildInfra{Build: datastore.KeyForObj(ctx, bld)}
		stp := &model.BuildSteps{Build: inf.Build}

		cancelSteps := true
		if err := datastore.Get(ctx, bld, inf, stp); err != nil {
			switch merr, ok := err.(errors.MultiError); {
			case !ok:
				return errors.Annotate(err, "failed to fetch build: %d", bld.ID).Err()
			case merr[0] == datastore.ErrNoSuchEntity:
				return perm.NotFoundErr(ctx)
			case merr[0] != nil:
				return errors.Annotate(merr[0], "failed to fetch build: %d", bld.ID).Err()
			case merr[1] != nil && merr[1] != datastore.ErrNoSuchEntity:
				return errors.Annotate(merr[1], "failed to fetch build infra: %d", bld.ID).Err()
			case merr[2] != nil && merr[2] != datastore.ErrNoSuchEntity:
				return errors.Annotate(merr[2], "failed to fetch build steps: %d", bld.ID).Err()
			case protoutil.IsEnded(bld.Proto.Status):
				return nil
			case merr[2] == datastore.ErrNoSuchEntity:
				cancelSteps = false
			}
		}
		if sw := inf.Proto.GetSwarming(); sw.GetHostname() != "" && sw.TaskId != "" {
			if err := tasks.CancelSwarmingTask(ctx, &taskdefs.CancelSwarmingTask{
				Hostname: sw.Hostname,
				TaskId:   sw.TaskId,
				Realm:    bld.Realm(),
			}); err != nil {
				return errors.Annotate(err, "failed to enqueue swarming task cancellation task: %d", bld.ID).Err()
			}
		}
		if rdb := inf.Proto.GetResultdb(); rdb.GetHostname() != "" && rdb.Invocation != "" {
			if err := tasks.FinalizeResultDB(ctx, &taskdefs.FinalizeResultDB{
				BuildId: bld.ID,
			}); err != nil {
				return errors.Annotate(err, "failed to enqueue resultdb finalization task: %d", bld.ID).Err()
			}
		}
		if err := tasks.ExportBigQuery(ctx, &taskdefs.ExportBigQuery{
			BuildId: bld.ID,
		}); err != nil {
			return errors.Annotate(err, "failed to enqueue bigquery export task: %d", bld.ID).Err()
		}
		if err := notify.NotifyPubSub(ctx, bld); err != nil {
			return errors.Annotate(err, "failed to enqueue pubsub notification task: %d", bld.ID).Err()
		}

		now := clock.Now(ctx).UTC()

		bld.Leasee = nil
		bld.LeaseExpirationDate = time.Time{}
		bld.LeaseKey = 0

		bld.Proto.CanceledBy = string(auth.CurrentIdentity(ctx))
		protoutil.SetStatus(now, bld.Proto, pb.Status_CANCELED)
		bld.Proto.SummaryMarkdown = summary

		toPut := []interface{}{bld}

		if cancelSteps {
			switch changed, err := stp.CancelIncomplete(ctx, timestamppb.New(now)); {
			case err != nil:
				return errors.Annotate(err, "failed to fetch build steps: %d", bld.ID).Err()
			case changed:
				toPut = append(toPut, stp)
			}
		}

		if err := datastore.Put(ctx, toPut...); err != nil {
			return errors.Annotate(err, "failed to store build: %d", bld.ID).Err()
		}
		return nil
	}, nil)
	if err != nil {
		return nil, err
	}

	return bld, nil
}
