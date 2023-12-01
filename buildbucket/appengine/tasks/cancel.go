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

package tasks

import (
	"context"
	"fmt"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	"go.chromium.org/luci/buildbucket/appengine/internal/perm"
	"go.chromium.org/luci/buildbucket/appengine/model"
	taskdefs "go.chromium.org/luci/buildbucket/appengine/tasks/defs"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

// StartCancel starts canceling a build and schedules the delayed task to finally cancel it
func StartCancel(ctx context.Context, bID int64, summary string) (*model.Build, error) {
	bld := &model.Build{ID: bID}
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		switch err := datastore.Get(ctx, bld); {
		case err == datastore.ErrNoSuchEntity:
			return perm.NotFoundErr(ctx)
		case err != nil:
			return errors.Annotate(err, "failed to fetch build: %d", bld.ID).Err()
		case protoutil.IsEnded(bld.Proto.Status):
			return nil
		case bld.Proto.CancelTime != nil:
			return nil
		}
		now := timestamppb.New(clock.Now(ctx).UTC())
		bld.Proto.CancelTime = now
		bld.Proto.UpdateTime = now
		bld.Proto.CancellationMarkdown = summary
		canceledBy := "buildbucket"
		if auth.CurrentIdentity(ctx) != identity.AnonymousIdentity {
			canceledBy = string(auth.CurrentIdentity(ctx))
		}

		bld.Proto.CanceledBy = canceledBy
		if err := datastore.Put(ctx, bld); err != nil {
			return errors.Annotate(err, "failed to store build: %d", bld.ID).Err()
		}
		// Enqueue the task to finally cancel the build.
		if err := ScheduleCancelBuildTask(ctx, bID, buildbucket.MinUpdateBuildInterval+bld.Proto.GracePeriod.AsDuration()); err != nil {
			return errors.Annotate(err, "failed to enqueue cancel task for build: %d", bld.ID).Err()
		}
		return nil
	}, nil)
	if err != nil {
		return bld, errors.Annotate(err, "failed to set the build to CANCELING: %d", bID).Err()
	}

	// TODO(crbug.com/1031205): alternatively, we could just map out the entire
	// cancellation tree and then feed those build IDs into a pool to do bulk
	// cancel. We could add a bool argument to StartCancel to control if the
	// function should cancel the entire tree or just the build itself.
	// Discussion: https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/3402796/comments/8aba3108_b4ca9f76
	if err := CancelChildren(ctx, bID); err != nil {
		// Failures of canceling children should not block canceling parent.
		logging.Debugf(ctx, "failed to cancel children of %d: %s", bID, err)
	}

	return bld, err
}

// CancelChildren cancels a build's children.
// NOTE: This process is best-effort; Builds call UpdateBuild at a minimum
// frequency and Buildbucket will inform them if they should start the cancel
// process (by checking the parent build, if any).
// So, even if this fails, the next UpdateBuild will catch it.
func CancelChildren(ctx context.Context, bID int64) error {
	// Look for the build's children to cancel.
	children, err := childrenToCancel(ctx, bID)
	if err != nil {
		return err
	}
	if len(children) == 0 {
		return nil
	}

	eg, ctx := errgroup.WithContext(ctx)
	summary := fmt.Sprintf("cancel since the parent %d is canceled", bID)

	for _, child := range children {
		child := child
		eg.Go(func() error {
			_, err := StartCancel(ctx, child.ID, summary)
			return err
		})
	}
	return eg.Wait()
}

// childrenToCancel returns the child build ids that should be canceled with
// the parent.
func childrenToCancel(ctx context.Context, bID int64) (children []*model.Build, err error) {
	q := datastore.NewQuery(model.BuildKind).Eq("parent_id", bID)
	err = datastore.Run(ctx, q, func(bld *model.Build) error {
		switch {
		case protoutil.IsEnded(bld.Proto.Status):
			return nil
		case bld.Proto.CanOutliveParent:
			return nil
		default:
			children = append(children, bld)
			return nil
		}
	})
	return
}

// Cancel actually cancels a build.
func Cancel(ctx context.Context, bID int64) (*model.Build, error) {
	bld := &model.Build{ID: bID}
	canceled := false
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		canceled = false // reset canceled in case of retries.
		inf := &model.BuildInfra{Build: datastore.KeyForObj(ctx, bld)}
		stp := &model.BuildSteps{Build: inf.Build}
		bs := &model.BuildStatus{Build: inf.Build}

		cancelSteps := true
		if err := datastore.Get(ctx, bld, inf, stp, bs); err != nil {
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
			case merr[3] != nil && merr[3] != datastore.ErrNoSuchEntity:
				// TODO(crbug.com/1430324): also check ErrNoSuchEntity.
				return errors.Annotate(merr[3], "failed to fetch build status: %d", bld.ID).Err()
			case merr[2] == datastore.ErrNoSuchEntity:
				cancelSteps = false
			}
		}
		if protoutil.IsEnded(bld.Proto.Status) {
			return nil
		}

		if sw := inf.Proto.GetSwarming(); sw.GetHostname() != "" && sw.TaskId != "" {
			if err := CancelSwarmingTask(ctx, &taskdefs.CancelSwarmingTaskGo{
				Hostname: sw.Hostname,
				TaskId:   sw.TaskId,
				Realm:    bld.Realm(),
			}); err != nil {
				return errors.Annotate(err, "failed to enqueue swarming task cancellation task: %d", bld.ID).Err()
			}
		}
		if bk := inf.Proto.GetBackend(); bk.GetTask().GetId().GetId() != "" && bk.GetTask().GetId().GetTarget() != "" {
			if err := CancelBackendTask(ctx, &taskdefs.CancelBackendTask{
				Target:  bk.Task.Id.Target,
				TaskId:  bk.Task.Id.Id,
				Project: bld.Project,
			}); err != nil {
				return errors.Annotate(err, "failed to enqueue backend task cancelation task: %d", bld.ID).Err()
			}
		}
		if rdb := inf.Proto.GetResultdb(); rdb.GetHostname() != "" && rdb.Invocation != "" {
			if err := FinalizeResultDB(ctx, &taskdefs.FinalizeResultDBGo{
				BuildId: bld.ID,
			}); err != nil {
				return errors.Annotate(err, "failed to enqueue resultdb finalization task: %d", bld.ID).Err()
			}
		}
		if err := ExportBigQuery(ctx, bld.ID, strings.Contains(bld.ExperimentsString(), buildbucket.ExperimentBqExporterGo)); err != nil {
			return errors.Annotate(err, "failed to enqueue bigquery export task: %d", bld.ID).Err()
		}
		if err := NotifyPubSub(ctx, bld); err != nil {
			return errors.Annotate(err, "failed to enqueue pubsub notification task: %d", bld.ID).Err()
		}

		now := clock.Now(ctx).UTC()

		bld.Leasee = nil
		bld.LeaseExpirationDate = time.Time{}
		bld.LeaseKey = 0

		protoutil.SetStatus(now, bld.Proto, pb.Status_CANCELED)
		logging.Debugf(ctx, fmt.Sprintf("Build %d status has now been set as canceled.", bld.ID))
		canceled = true
		toPut := []any{bld}

		if bs.Status != pb.Status_STATUS_UNSPECIFIED {
			bs.Status = pb.Status_CANCELED
			toPut = append(toPut, bs)
		}

		if cancelSteps {
			switch changed, err := stp.CancelIncomplete(ctx, timestamppb.New(now)); {
			case err != nil:
				return errors.Annotate(err, "failed to mark steps cancelled: %d", bld.ID).Err()
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
	if protoutil.IsEnded(bld.Status) && canceled {
		metrics.BuildCompleted(ctx, bld)
	}

	return bld, nil
}

// ScheduleCancelBuildTask enqueues a CancelBuildTask.
func ScheduleCancelBuildTask(ctx context.Context, bID int64, delay time.Duration) error {
	return tq.AddTask(ctx, &tq.Task{
		Title: fmt.Sprintf("cancel-%d", bID),
		Payload: &taskdefs.CancelBuildTask{
			BuildId: bID,
		},
		Delay: delay,
	})
}
