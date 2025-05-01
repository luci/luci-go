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

package tasks

import (
	"context"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/buildbucket/appengine/internal/buildstatus"
	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	"go.chromium.org/luci/buildbucket/appengine/model"
	taskdefs "go.chromium.org/luci/buildbucket/appengine/tasks/defs"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

// sendOnBuildCompletion sends a bunch of related events when build is reaching
// to an end status, e.g. finalizing the resultdb invocation, exporting to Bq,
// and notify pubsub topics.
func sendOnBuildCompletion(ctx context.Context, bld *model.Build, inf *model.BuildInfra) error {
	bld.ClearLease()

	return parallel.FanOutIn(func(tks chan<- func() error) {
		tks <- func() error {
			return errors.WrapIf(NotifyPubSub(ctx, bld), "failed to enqueue pubsub notification task: %d", bld.ID)
		}
		tks <- func() error {
			return errors.WrapIf(ExportBigQuery(ctx, bld.ID), "failed to enqueue bigquery export task: %d", bld.ID)
		}
		tks <- func() error {
			bldr := &model.Builder{
				ID:     bld.Proto.Builder.Builder,
				Parent: model.BucketKey(ctx, bld.Proto.Builder.Project, bld.Proto.Builder.Bucket),
			}
			// Get the builder out of transaction to avoid "concurrent transaction"
			// errors with too many concurrent reads on a busy builder.
			if err := datastore.Get(datastore.WithoutTransaction(ctx), bldr); err != nil {
				if errors.Is(err, datastore.ErrNoSuchEntity) {
					// Builder not found. Could be
					// * The build runs in a dynamic builder,
					// * The builder is deleted while the build is still running.
					// In either case it's fine to bypass checking max current builds.
					return nil
				}
				return err
			}
			if bldr.Config.GetMaxConcurrentBuilds() > 0 {
				return errors.WrapIf(CreatePopPendingBuildTask(ctx, &taskdefs.PopPendingBuildTask{
					BuildId:   bld.ID,
					BuilderId: bld.Proto.Builder,
				}, ""), "failed to enqueue pop pending build task: %d", bld.ID)
			}
			return nil
		}
		tks <- func() error {
			if inf == nil {
				inf = &model.BuildInfra{Build: datastore.KeyForObj(ctx, bld)}
				if err := datastore.Get(ctx, inf); err != nil {
					return err
				}
			}
			if rdb := inf.Proto.GetResultdb(); rdb.GetHostname() != "" && rdb.Invocation != "" {
				return errors.WrapIf(FinalizeResultDB(ctx, &taskdefs.FinalizeResultDBGo{BuildId: bld.ID}), "failed to enqueue resultDB finalization task: %d", bld.ID)
			}
			return nil // no-op
		}
	})
}

// SendOnBuildStatusChange sends cloud tasks if a build's top level status changes.
//
// It's the default PostProcess func for buildstatus.Updater.
//
// Must run in a datastore transaction.
func SendOnBuildStatusChange(ctx context.Context, bld *model.Build, inf *model.BuildInfra) error {
	if datastore.Raw(ctx) == nil || datastore.CurrentTransaction(ctx) == nil {
		return errors.Reason("must enqueue cloud tasks that are triggered by build status update in a transaction").Err()
	}
	switch {
	case bld.Proto.Status == pb.Status_STARTED:
		if err := NotifyPubSub(ctx, bld); err != nil {
			logging.Debugf(ctx, "failed to notify pubsub about starting %d: %s", bld.ID, err)
		}
	case protoutil.IsEnded(bld.Proto.Status):
		return sendOnBuildCompletion(ctx, bld, inf)
	}
	return nil
}

// failBuild fails the given build with INFRA_FAILURE status.
func failBuild(ctx context.Context, buildID int64, msg string) error {
	bld := &model.Build{
		ID: buildID,
	}

	statusUpdated := false
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		switch err := datastore.Get(ctx, bld); {
		case err == datastore.ErrNoSuchEntity:
			logging.Warningf(ctx, "build %d not found: %s", buildID, err)
			return nil
		case err != nil:
			return errors.Annotate(err, "failed to fetch build: %d", bld.ID).Err()
		}

		if protoutil.IsEnded(bld.Proto.Status) {
			// Build already ended, no more change to it.
			return nil
		}

		statusUpdated = true
		bld.Proto.SummaryMarkdown = msg
		st := &buildstatus.StatusWithDetails{Status: pb.Status_INFRA_FAILURE}
		bs, steps, err := updateBuildStatusOnTaskStatusChange(ctx, bld, nil, st, st, clock.Now(ctx), false)
		if err != nil {
			return err
		}

		toSave := []any{bld}
		if bs != nil {
			toSave = append(toSave, bs)
		}
		if steps != nil {
			toSave = append(toSave, steps)
		}
		return datastore.Put(ctx, toSave)
	}, nil)
	if err != nil {
		return transient.Tag.Apply(errors.Annotate(err, "failed to terminate build: %d", buildID).Err())
	}
	if statusUpdated {
		metrics.BuildCompleted(ctx, bld)
	}
	return nil
}

// updateBuildStatusOnTaskStatusChange updates build's top level status based on
// task status change.
func updateBuildStatusOnTaskStatusChange(ctx context.Context, bld *model.Build, inf *model.BuildInfra, buildStatus, taskStatus *buildstatus.StatusWithDetails, updateTime time.Time, useTaskSuccess bool) (*model.BuildStatus, *model.BuildSteps, error) {
	var steps *model.BuildSteps
	statusUpdater := buildstatus.Updater{
		Build:                       bld,
		BuildStatus:                 buildStatus,
		TaskStatus:                  taskStatus,
		UpdateTime:                  updateTime,
		SucceedBuildIfTaskSucceeded: useTaskSuccess,
		PostProcess: func(c context.Context, bld *model.Build, inf *model.BuildInfra) error {
			// Besides the post process cloud tasks, we also need to update
			// steps, in case the build task ends before the build does.
			if protoutil.IsEnded(bld.Proto.Status) {
				steps = &model.BuildSteps{Build: datastore.KeyForObj(ctx, bld)}
				// If the build has no steps, CancelIncomplete will return false.
				if err := model.GetIgnoreMissing(ctx, steps); err != nil {
					return errors.Annotate(err, "failed to fetch steps for build %d", bld.ID).Err()
				}
				switch _, err := steps.CancelIncomplete(ctx, timestamppb.New(updateTime.UTC())); {
				case err != nil:
					// The steps are fetched from datastore and should always be valid in
					// CancelIncomplete. But in case of any errors, we can just log it here
					// instead of rethrowing it to make the entire flow fail or retry.
					logging.Errorf(ctx, "failed to mark steps cancelled for build %d: %s", bld.ID, err)
				}
			}
			return SendOnBuildStatusChange(ctx, bld, inf)
		},
	}
	bs, err := statusUpdater.Do(ctx)
	if err != nil {
		return nil, nil, err
	}
	return bs, steps, err
}
