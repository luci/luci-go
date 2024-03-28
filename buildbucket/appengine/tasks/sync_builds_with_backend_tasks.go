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
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/buildbucket/appengine/internal/clients"
	"go.chromium.org/luci/buildbucket/appengine/internal/config"
	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

// Batch size to fetch tasks from backend.
var fetchBatchSize = 1000

// Batch size to update builds and sub entities in on transaction.
// As of Mar 2024, our database is on Firestore in Datastore mode, using
// Optimistic With Entity Groups concurrency mode.
// Transactions are limited to 25 entity groups.
// Consider each build and it's sub-entities as one group, and if the build
// is ended, we'll need to submit 3 transactional tasks for it (bq-export,
// resultdb-finalization and pubsub), so in total 4 entity groups.
// Theoritically that means we could have 25/4 = 6 in one batch, make it 5
// for extra buffer room.
var updateBatchSize = 5

// queryBuildsToSync runs queries to get incomplete builds from the project running
// on the backend that have reached/exceeded their next sync time.
//
// It will run n parallel queries where n is the number of shards for the backend.
// The queries pass the results to bkC for post process.
func queryBuildsToSync(ctx context.Context, mr parallel.MultiRunner, backend, project string, shards int32, now time.Time, bkC chan []*datastore.Key) error {
	baseQ := datastore.NewQuery(model.BuildKind).Eq("incomplete", true).Eq("backend_target", backend).Eq("project", project)

	return mr.RunMulti(func(work chan<- func() error) {
		for i := 0; i < int(shards); i++ {
			i := i
			work <- func() error {
				bks := make([]*datastore.Key, 0, fetchBatchSize)
				left := model.ConstructNextSyncTime(backend, project, i, time.Time{})
				right := model.ConstructNextSyncTime(backend, project, i, now)
				q := baseQ.Lt("next_backend_sync_time", right).Gt("next_backend_sync_time", left)
				err := datastore.RunBatch(ctx, int32(fetchBatchSize), q.KeysOnly(true),
					func(bk *datastore.Key) error {
						bks = append(bks, bk)
						if len(bks) == fetchBatchSize {
							bkC <- bks
							bks = make([]*datastore.Key, 0, fetchBatchSize)
						}
						return nil
					},
				)
				if len(bks) > 0 {
					bkC <- bks
				}
				return err
			}
		}
	})
}

type buildAndInfra struct {
	build *model.Build
	infra *model.BuildInfra
}

func buildHasBeenUpdated(b *model.Build, now time.Time) bool {
	_, _, _, nextSync := b.MustParseNextBackendSyncTime()
	nowUnix := fmt.Sprint(now.Truncate(time.Minute).Unix())
	return nextSync > nowUnix
}
func getEntities(ctx context.Context, bks []*datastore.Key, now time.Time) ([]*buildAndInfra, error) {
	var blds []*model.Build
	var infs []*model.BuildInfra
	var toGet []any
	for _, k := range bks {
		b := &model.Build{}
		populated := datastore.PopulateKey(b, k)
		if !populated {
			continue
		}
		inf := &model.BuildInfra{Build: k}
		blds = append(blds, b)
		infs = append(infs, inf)
		toGet = append(toGet, b, inf)
	}
	if err := datastore.Get(ctx, toGet...); err != nil {
		return nil, errors.Annotate(err, "error fetching builds %q", bks).Err()
	}

	var entitiesToSync []*buildAndInfra
	for i, bld := range blds {
		inf := infs[i]
		switch {
		case bld == nil || inf == nil:
			continue
		case protoutil.IsEnded(bld.Status):
			continue
		case inf.Proto.GetBackend().GetTask().GetId().GetId() == "":
			// No task is associated to the build, log the error but move on.
			logging.Errorf(ctx, "build %d does not have backend task associated", bld.ID)
			continue
		case buildHasBeenUpdated(bld, now):
			// Build has been updated, skip.
			continue
		}
		entitiesToSync = append(entitiesToSync, &buildAndInfra{build: bld, infra: inf})
	}
	return entitiesToSync, nil
}

func updateEntities(ctx context.Context, bks []*datastore.Key, now time.Time, taskMap map[string]*pb.Task) ([]*model.Build, error) {
	var endedBld []*model.Build
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		entities, err := getEntities(ctx, bks, now)
		switch {
		case err != nil:
			return err
		case len(entities) == 0:
			// Nothing to sync.
			return nil
		}
		logging.Infof(ctx, "updating %d builds with their backend tasks", len(bks))

		var toPut []any
		for _, ent := range entities {
			bld := ent.build
			inf := ent.infra
			t := inf.Proto.Backend.GetTask()
			taskID := t.GetId().GetId()
			if taskID == "" {
				// impossible.
				logging.Errorf(ctx, "failed to get backend task id for build %d", bld.ID)
				continue
			}
			fetchedTask := taskMap[taskID]
			switch {
			case fetchedTask == nil:
				logging.Errorf(ctx, "backend task %s:%s is not in valid fetched tasks", t.GetId().GetTarget(), taskID)
				continue
			case fetchedTask.UpdateId < t.UpdateId:
				logging.Errorf(ctx, "FetchTasks returns stale task for %s:%s with update_id %d, which task in datastore has update_id %d", t.GetId().GetTarget(), taskID, fetchedTask.UpdateId, t.UpdateId)
				continue
			case fetchedTask.UpdateId == t.UpdateId:
				// No update from the task, so it's still running.
				// Update build's UpdateTime (so that NextBackendSyncTime is
				// recalculated when save) and we're done.
				bld.Proto.UpdateTime = timestamppb.New(clock.Now(ctx))
				toPut = append(toPut, bld)
				continue
			}
			toSave, err := prepareUpdate(ctx, bld, inf, fetchedTask)
			if err != nil {
				logging.Errorf(ctx, "failed to update task for build %d: %s", bld.ID, err)
				continue
			}
			toPut = append(toPut, toSave...)

			if protoutil.IsEnded(fetchedTask.Status) {
				endedBld = append(endedBld, bld)
			}
		}
		return datastore.Put(ctx, toPut)
	}, nil)
	if err != nil {
		logging.Errorf(ctx, "transaction error: %s", err)
	}
	return endedBld, err
}

// validateResponses iterates through FetchTaskResponse.Responses and logs the
// taskIDs that returned with errors and returns a map of taskIDs to valid tasks.
func validateResponses(ctx context.Context, responses []*pb.FetchTasksResponse_Response, numTaskIDsRequsted int) (map[string]*pb.Task, errors.MultiError) {
	if len(responses) != numTaskIDsRequsted {
		return nil, errors.NewMultiError(errors.New(fmt.Sprintf("FetchTasksResponse returned with %d responses when %d were requested", len(responses), numTaskIDsRequsted)))
	}
	var err errors.MultiError
	taskMap := map[string]*pb.Task{}
	var validTaskIDs []string
	fetchedCount := 0
	for idx, resp := range responses {
		switch r := resp.Response.(type) {
		case *pb.FetchTasksResponse_Response_Task:
			fetchedCount += 1
			if e := validateTask(r.Task, true); e != nil {
				err.MaybeAdd(e)
				continue
			}
			taskMap[resp.GetTask().Id.GetId()] = resp.GetTask()
			validTaskIDs = append(validTaskIDs, resp.GetTask().Id.GetId())
		case *pb.FetchTasksResponse_Response_Error:
			status := resp.GetError()
			err.MaybeAdd(errors.New(fmt.Sprintf("Error at index %d: %d-%s", idx, status.Code, status.Message)))
		}
	}
	// TODO(crbug.com/1472896): Remove the log after confirming the build task
	// sync cron WAI.
	if len(validTaskIDs) > 0 {
		logging.Infof(ctx, "requested %d tasks, fetched %d, valid %d: %q", len(responses), fetchedCount, len(validTaskIDs), validTaskIDs)
	}
	return taskMap, err
}

// syncBuildsWithBackendTasks fetches backend tasks for the builds of a project,
// then updates the builds.
//
// The task only retries if there's top level errors. In the case that a single
// build is failed to update, we'll wait for the next task to update it again.
func syncBuildsWithBackendTasks(ctx context.Context, mr parallel.MultiRunner, bc *clients.BackendClient, bks []*datastore.Key, now time.Time) error {
	if len(bks) == 0 {
		return nil
	}

	entities, err := getEntities(ctx, bks, now)
	switch {
	case err != nil:
		return err
	case len(entities) == 0:
		// Nothing to sync.
		return nil
	}

	// Fetch backend tasks.
	var taskIDs []*pb.TaskID
	for _, ent := range entities {
		taskIDs = append(taskIDs, ent.infra.Proto.Backend.Task.Id)
	}
	if len(taskIDs) == 0 {
		return nil
	}
	// TODO(crbug.com/1472896): Simplify the log after confirming the build task
	// sync cron WAI.
	logging.Infof(ctx, "Fetching %d backend tasks %q", len(taskIDs), taskIDs)
	resp, err := bc.FetchTasks(ctx, &pb.FetchTasksRequest{TaskIds: taskIDs})
	if err != nil {
		return errors.Annotate(err, "failed to fetch backend tasks").Err()
	}

	// Validate fetched tasks and create a task map with validated tasks.
	taskMap, errs := validateResponses(ctx, resp.Responses, len(taskIDs))
	if errs.First() != nil {
		logging.Errorf(ctx, errs.AsError().Error())
		// Return early since taskMap must be empty.
		if len(errs) == len(taskIDs) {
			return errs
		}
	}

	// Update entities for the builds that need to sync.
	curBatch := make([]*datastore.Key, 0, updateBatchSize)
	var bksBatchesToSync [][]*datastore.Key
	for _, ent := range entities {
		curBatch = append(curBatch, datastore.KeyForObj(ctx, ent.build))
		if len(curBatch) == updateBatchSize {
			bksBatchesToSync = append(bksBatchesToSync, curBatch)
			curBatch = make([]*datastore.Key, 0, updateBatchSize)
		}
	}
	if len(curBatch) > 0 {
		bksBatchesToSync = append(bksBatchesToSync, curBatch)
	}
	var endedBld []*model.Build
	for _, batch := range bksBatchesToSync {
		batch := batch
		err := mr.RunMulti(func(work chan<- func() error) {
			work <- func() error {
				endedBldInBatch, txErr := updateEntities(ctx, batch, now, taskMap)
				if txErr != nil {
					return transient.Tag.Apply(errors.Annotate(err, "failed to sync backend tasks").Err())
				}
				endedBld = append(endedBld, endedBldInBatch...)
				return nil
			}
		})
		if err != nil {
			return err
		}
	}

	for _, b := range endedBld {
		metrics.BuildCompleted(ctx, b)
	}

	return nil
}

// SyncBuildsWithBackendTasks syncs all the builds belongs to `project` running
// on `backend` with their backend tasks if their next sync time have been
// exceeded.
func SyncBuildsWithBackendTasks(ctx context.Context, backend, project string) error {
	globalCfg, err := config.GetSettingsCfg(ctx)
	if err != nil {
		return errors.Annotate(err, "could not get global settings config").Err()
	}

	var shards int32
	backendFound := false
	for _, config := range globalCfg.Backends {
		if config.Target == backend {
			if config.GetFullMode() == nil {
				// No need to sync tasks if it's not in a full mode.
				return nil
			}
			backendFound = true
			shards = config.GetFullMode().GetBuildSyncSetting().GetShards()
		}
	}
	if !backendFound {
		return tq.Fatal.Apply(errors.Reason("failed to find backend %s from global config", backend).Err())
	}

	bc, err := clients.NewBackendClient(ctx, project, backend, globalCfg)
	if err != nil {
		return tq.Fatal.Apply(errors.Annotate(err, "failed to connect to backend service %s as project %s", backend, project).Err())
	}

	now := clock.Now(ctx)
	if shards == 0 {
		shards = 1
	}
	nWorkers := int(shards)
	finalErr := parallel.RunMulti(ctx, nWorkers, func(mr parallel.MultiRunner) error {
		return mr.RunMulti(func(work chan<- func() error) {
			bkC := make(chan []*datastore.Key)

			work <- func() error {
				defer close(bkC)
				return queryBuildsToSync(ctx, mr, backend, project, shards, now, bkC)
			}

			for bks := range bkC {
				bks := bks
				work <- func() error {
					return syncBuildsWithBackendTasks(ctx, mr, bc, bks, now)
				}
			}
		})
	})
	if finalErr != nil {
		logging.Errorf(ctx, "final error of the task: %s", finalErr)
	}
	return finalErr
}
