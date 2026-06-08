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
	"time"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/buildbucket/appengine/internal/buildstatus"
	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	"go.chromium.org/luci/buildbucket/appengine/internal/perm"
	"go.chromium.org/luci/buildbucket/appengine/model"
	taskdefs "go.chromium.org/luci/buildbucket/appengine/tasks/defs"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

// CancellationDetails are passed to StartCancel.
type CancellationDetails struct {
	// CanceledBy is what to put in Build's `canceled_by` field.
	CanceledBy string
	// Summary is what to put in Build's `cancellation_markdown` field.
	Summary string
	// SkipGracePeriod instructs to kill the build ASAP.
	SkipGracePeriod bool
}

// StartCancel moves the build into canceling state and schedules the delayed
// task to definitely terminate the build upon reaching the grace termination
// timeout.
//
// Performs best effort cancellation of all transitive child builds not marked
// as CanOutliveParent. Does it even if the root build is already finished or
// is already being cancelled.
//
// Returns the build as it is in the datastore after this call. Can be nil if
// it is missing.
//
// Returns appstatus errors.
func StartCancel(ctx context.Context, bID int64, details *CancellationDetails) (*model.Build, error) {
	var bld *model.Build

	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		bld = &model.Build{ID: bID}
		switch err := datastore.Get(ctx, bld); {
		case err == datastore.ErrNoSuchEntity:
			bld = nil
			return perm.NotFoundErr(ctx)
		case err != nil:
			return appstatus.Errorf(codes.Internal, "failed to fetch build: %d: %s", bld.ID, err)
		case protoutil.IsEnded(bld.Proto.Status):
			return nil
		case bld.Proto.CancelTime != nil:
			return nil
		}

		// Mark the build as being canceled. The bbagent should notice that on its
		// next heartbeat call and start actual cancellation.
		now := timestamppb.New(clock.Now(ctx).UTC())
		bld.Proto.CancelTime = now
		bld.Proto.UpdateTime = now
		bld.Proto.CanceledBy = details.CanceledBy
		bld.Proto.CancellationMarkdown = details.Summary
		if err := datastore.Put(ctx, bld); err != nil {
			return appstatus.Errorf(codes.Internal, "failed to store build: %d: %s", bld.ID, err)
		}

		// Enqueue the task to cancel the build upon reaching the deadline in case
		// the bbagent is not responding to seeing the build is being cancelled.
		//
		// TODO: Skip waiting if the build hasn't started yet. There's no sense in
		// waiting for GracePeriod if the build is still queued, since bbagent isn't
		// running yet and it can't possibly do graceful termination.
		delay := buildbucket.MinUpdateBuildInterval + bld.Proto.GracePeriod.AsDuration()
		if details.SkipGracePeriod {
			delay = 0
		}
		if err := ScheduleCancelBuildTask(ctx, bID, delay); err != nil {
			return appstatus.Errorf(codes.Internal, "failed to enqueue cancel task for build: %d: %s", bld.ID, err)
		}

		return nil
	}, nil)
	if err != nil {
		return bld, err
	}

	// TODO(crbug.com/1031205): alternatively, we could just map out the entire
	// cancellation tree and then feed those build IDs into a pool to do bulk
	// cancel. We could add a bool argument to StartCancel to control if the
	// function should cancel the entire tree or just the build itself.
	// Discussion: https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/3402796/comments/8aba3108_b4ca9f76
	if err := CancelChildren(ctx, bID, details); err != nil {
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
func CancelChildren(ctx context.Context, bID int64, details *CancellationDetails) error {
	// Look for the build's children to cancel.
	children, err := childrenToCancel(ctx, bID)
	if err != nil {
		return err
	}
	if len(children) == 0 {
		return nil
	}

	updated := *details
	updated.Summary = fmt.Sprintf("Cancel since the parent %d is canceled", bID)
	updated.SkipGracePeriod = false

	var eg errgroup.Group
	eg.SetLimit(64)

	merr := make(errors.MultiError, len(children))
	for idx, child := range children {
		eg.Go(func() error {
			_, merr[idx] = StartCancel(ctx, child.ID, &updated)
			return nil // carry on cancelling
		})
	}
	eg.Wait()

	return merr.First()
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

// Cancel cancels a build task associated with the build.
//
// This is called as a delayed CancelBuildTask when reaching the grace timeout.
func Cancel(ctx context.Context, bID int64) (*model.Build, error) {
	bld := &model.Build{ID: bID}
	canceled := false
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		canceled = false // reset canceled in case of retries.
		inf := &model.BuildInfra{Build: datastore.KeyForObj(ctx, bld)}
		stp := &model.BuildSteps{Build: inf.Build}

		cancelSteps := true
		if err := datastore.Get(ctx, bld, inf, stp); err != nil {
			switch merr, ok := err.(errors.MultiError); {
			case !ok:
				return errors.Fmt("failed to fetch build: %d: %w", bld.ID, err)
			case merr[0] == datastore.ErrNoSuchEntity:
				return perm.NotFoundErr(ctx)
			case merr[0] != nil:
				return errors.Fmt("failed to fetch build: %d: %w", bld.ID, merr[0])
			case merr[1] != nil && merr[1] != datastore.ErrNoSuchEntity:
				return errors.Fmt("failed to fetch build infra: %d: %w", bld.ID, merr[1])
			case merr[2] != nil && merr[2] != datastore.ErrNoSuchEntity:
				return errors.Fmt("failed to fetch build steps: %d: %w", bld.ID, merr[2])
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
				return errors.Fmt("failed to enqueue swarming task cancellation task: %d: %w", bld.ID, err)
			}
		}
		if bk := inf.Proto.GetBackend(); bk.GetTask().GetId().GetId() != "" && bk.GetTask().GetId().GetTarget() != "" {
			if err := CancelBackendTask(ctx, &taskdefs.CancelBackendTask{
				Target:  bk.Task.Id.Target,
				TaskId:  bk.Task.Id.Id,
				Project: bld.Project,
			}); err != nil {
				return errors.Fmt("failed to enqueue backend task cancelation task: %d: %w", bld.ID, err)
			}
		}

		now := clock.Now(ctx).UTC()

		bld.Leasee = nil
		bld.LeaseExpirationDate = time.Time{}
		bld.LeaseKey = 0

		canceled = true
		toPut := []any{bld}
		statusUpdater := buildstatus.Updater{
			Build:       bld,
			Infra:       inf,
			BuildStatus: &buildstatus.StatusWithDetails{Status: pb.Status_CANCELED},
			UpdateTime:  now,
			PostProcess: SendOnBuildStatusChange,
		}
		bs, err := statusUpdater.Do(ctx)
		if err != nil {
			return errors.Fmt("failed to set status for build %d: %w", bld.ID, err)
		}
		if bs != nil {
			toPut = append(toPut, bs)
		}

		if cancelSteps {
			switch changed, err := stp.CancelIncomplete(ctx, timestamppb.New(now)); {
			case err != nil:
				return errors.Fmt("failed to mark steps cancelled: %d: %w", bld.ID, err)
			case changed:
				toPut = append(toPut, stp)
			}
		}

		if err := datastore.Put(ctx, toPut...); err != nil {
			return errors.Fmt("failed to store build: %d: %w", bld.ID, err)
		}
		return nil
	}, nil)
	if err != nil {
		return nil, err
	}
	if protoutil.IsEnded(bld.Status) && canceled {
		logging.Infof(ctx, fmt.Sprintf("Build %d status has now been set as canceled.", bld.ID))
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
