// Copyright 2024 The LUCI Authors.
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
	"encoding/hex"
	"sort"
	"time"

	"github.com/google/uuid"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/cfg"
	"go.chromium.org/luci/swarming/server/model"
)

// ErrAlreadyExists is a special error to return when task ID collision happens.
var ErrAlreadyExists = errors.New("task already exists")

// Creation contains information to create a new task.
type Creation struct {
	// RequestID is used to make new task request idempotent.
	RequestID string

	// Request is the TaskRequest entity representing the new task.
	Request *model.TaskRequest
	// SecretBytes is the SecretBytes entity.
	SecretBytes *model.SecretBytes
	// BuildTask is the BuildTask entity.
	BuildTask *model.BuildTask

	// SwarmingProject is the Cloud project of the Swarming service, e.g. "chromium-swarm".
	SwarmingProject string

	// ServerVersion is the version of the executing binary.
	ServerVersion string

	// Config is a snapshot of the server configuration.
	Config *cfg.Config

	// LifecycleTasks is used to emit TQ tasks related to Swarming task lifecycle.
	LifecycleTasks LifecycleTasks
}

// Run creates and stores all the entities to create a new task.
//
// The number of entities created is ~5: TaskRequest, TaskToRunShard and
// TaskResultSummary and (optionally) SecretBytes and BuildTask. They are in
// single entity group and saved in a single transaction.
// If c.RequestID is provided, a TaskRequestID may also be created if
// c.RequestID is being used the first time.
//
// If task ID collision happens, a special error `ErrAlreadyExists` will be
// returned so the server could retry creating the entities.
func (c *Creation) Run(ctx context.Context) (*model.TaskResultSummary, error) {
	trs, err := c.dedupByRequestID(ctx)
	switch {
	case err != nil:
		return nil, err
	case trs != nil:
		return trs, nil
	}

	// Populate TaskResultSummary and entity keys.
	// Make a shallow copy of provided entities since we are going to
	// modify them. Modifying them in-place could cause bugs if we retry this
	// creation when ID collision happens.
	trv := *c.Request
	tr := &trv
	var sb *model.SecretBytes
	if c.SecretBytes != nil {
		sbv := *c.SecretBytes
		sb = &sbv
	}
	var bt *model.BuildTask
	if c.BuildTask != nil {
		btv := *c.BuildTask
		bt = &btv
	}

	tr.Key = model.NewTaskRequestKey(ctx)
	taskID := model.RequestKeyToTaskID(tr.Key, model.AsRequest)
	tr.TxnUUID = uuid.New().String()
	now := clock.Now(ctx)
	trs = model.NewTaskResultSummary(ctx, tr, c.ServerVersion, now)
	if sb != nil {
		sb.Key = model.SecretBytesKey(ctx, tr.Key)
	}
	if bt != nil {
		bt.Key = model.BuildTaskKey(ctx, tr.Key)
	}

	// Precalculate all properties hashes in advance. That way even if we end up
	// using e.g. first task slice, all hashes will still be populated (for BQ
	// export).
	for i := range len(tr.TaskSlices) {
		s := &tr.TaskSlices[i]
		if err := s.PrecalculatePropertiesHash(sb); err != nil {
			return nil, errors.Annotate(err, "error calculating properties hash for slice %d", i).Err()
		}
	}

	// Dedup by properties hashes.
	var dupResult *model.TaskResultSummary
	for i, s := range tr.TaskSlices {
		if !s.Properties.Idempotent {
			continue
		}
		dupResult, err = c.findDuplicateTask(ctx, s.PropertiesHash)
		if err != nil {
			return nil, err
		}
		if dupResult != nil {
			c.copyDuplicateTask(ctx, trs, dupResult, i)
			// There's not much to do as the task will not run, previous
			// results are returned. We still need to store the TaskRequest
			// and TaskResultSummary.
			// Since the has_secret_bytes/has_build_task property is already set
			// for UI purposes, and the task itself will never run, we skip
			// storing the SecretBytes/BuildTask, as they would never be read
			// and will just consume space in the datastore (and the task we
			// deduplicate with will have them stored anyway, if we really want
			// to get them again).
			sb = nil
			bt = nil
			break
		}
	}

	var ttr *model.TaskToRun
	if dupResult == nil {
		// The task has to run.
		// Start with zeroth slice. If there are slices that can't execute due
		// to missing bots, there will be a ping pong game between Swarming and
		// RBE skipping them.
		trs.CurrentTaskSlice = 0
		ttr, err = model.NewTaskToRun(ctx, c.SwarmingProject, tr, int(trs.CurrentTaskSlice))
		if err != nil {
			return nil, err
		}
	}
	// TODO(b/355013250): Create ResultDB invocation.

	err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		// Recheck TaskRequestID in transaction in case it has just been
		// created after previous check.
		dupTrs, err := c.dedupByRequestID(ctx)
		switch {
		case err != nil:
			return err
		case dupTrs != nil:
			trs = dupTrs
			return nil
		}

		// Check ID collision.
		existing := &model.TaskRequest{
			Key: tr.Key,
		}
		switch err = datastore.Get(ctx, existing); {
		case err == nil:
			if existing.TxnUUID == tr.TxnUUID {
				// Entities have been saved, nothing left to do.
				return nil
			}
			// ID collision encountered, should be rare.
			logging.Errorf(ctx, "Task ID collision: %s already exists", taskID)
			return ErrAlreadyExists
		case !errors.Is(err, datastore.ErrNoSuchEntity):
			return err
		}

		toPut := []any{
			tr,
			trs,
		}
		if sb != nil {
			toPut = append(toPut, sb)
		}
		if bt != nil {
			toPut = append(toPut, bt)
		}

		// c.RequestID is being used the first time, create a new
		// TaskRequestID entity for it.
		if c.RequestID != "" {
			tri := &model.TaskRequestID{
				Key:      model.TaskRequestIDKey(ctx, c.RequestID),
				TaskID:   taskID,
				ExpireAt: now.Add(time.Hour * 24 * 7),
			}
			toPut = append(toPut, tri)
		}

		if ttr != nil {
			toPut = append(toPut, ttr)
			if err := c.LifecycleTasks.enqueueRBENew(ctx, tr, ttr); err != nil {
				return err
			}
		}

		// TODO(b/355013510): handle BuildTask

		if trs.State != apipb.TaskState_PENDING {
			if err := c.LifecycleTasks.sendOnTaskUpdate(ctx, tr, trs); err != nil {
				return errors.Annotate(err,
					"failed to enqueue pubsub notification cloud tasks for creating task %s", taskID).Err()
			}
		}
		return datastore.Put(ctx, toPut...)
	}, nil)
	if err != nil {
		return nil, errors.Annotate(err, "error saving the task").Err()
	}

	if dupResult != nil {
		logging.Infof(ctx, "New request %s reusing %s", taskID,
			model.RequestKeyToTaskID(dupResult.TaskRequestKey(), model.AsRequest))
	}

	onTaskRequested(ctx, trs, dupResult != nil)
	return trs, nil
}

func (c *Creation) dedupByRequestID(ctx context.Context) (*model.TaskResultSummary, error) {
	if c.RequestID == "" {
		return nil, nil
	}
	tri := &model.TaskRequestID{
		Key: model.TaskRequestIDKey(ctx, c.RequestID),
	}
	switch err := datastore.Get(ctx, tri); {
	case err == nil:
		trs, subErr := model.TaskResultSummaryFromID(ctx, tri.TaskID)
		if subErr != nil {
			return nil, errors.Annotate(subErr, "failed to get TaskResultSummary for request id %s", c.RequestID).Err()
		}
		return trs, nil
	case !errors.Is(err, datastore.ErrNoSuchEntity):
		return nil, errors.Annotate(err, "failed to get TaskRequestID for request id %s", c.RequestID).Err()
	default:
		return nil, nil
	}
}

// findDuplicateTask finds a previously run task that can be reused.
//
// See TaskResultSummary.PropertiesHash on what tasks are reusable.
func (c *Creation) findDuplicateTask(ctx context.Context, propertiesHash []byte) (*model.TaskResultSummary, error) {
	logging.Infof(ctx, "Look for duplicate task with properties_hash %s", hex.EncodeToString(propertiesHash))
	var results []*model.TaskResultSummary
	q := model.TaskResultSummaryQuery().
		Eq("properties_hash", propertiesHash).
		Order("__key__").
		Limit(1)
	if err := datastore.GetAll(ctx, q, &results); err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}

	res := results[0]
	taskReusableFor := time.Duration(c.Config.Settings().GetReusableTaskAgeSecs()) * time.Second
	if res.Created.Before(clock.Now(ctx).Add(-taskReusableFor)) {
		logging.Infof(ctx,
			"Found duplicate task %s is older than %s, skipping",
			model.RequestKeyToTaskID(res.TaskRequestKey(), model.AsRunResult), taskReusableFor)
		return nil, nil
	}
	return res, nil
}

// copyDuplicateTask copies the selected attributes of entity dup into new.
func (c *Creation) copyDuplicateTask(ctx context.Context, new, dup *model.TaskResultSummary, curSlice int) {
	// Copy from dup.
	new.State = dup.State
	new.BotVersion = dup.BotVersion
	new.BotDimensions = dup.BotDimensions
	new.BotIdleSince = dup.BotIdleSince
	new.BotLogsCloudProject = dup.BotLogsCloudProject
	new.Started = dup.Started
	new.ExitCode = dup.ExitCode
	new.Completed = dup.Completed
	new.ExitCode = dup.ExitCode
	new.StdoutChunks = dup.StdoutChunks
	new.CASOutputRoot = dup.CASOutputRoot
	new.CIPDPins = dup.CIPDPins
	new.ResultDBInfo = dup.ResultDBInfo
	new.BotID = dup.BotID
	new.RequestPriority = dup.RequestPriority
	new.RequestRealm = dup.RequestRealm
	new.RequestAuthenticated = dup.RequestAuthenticated
	new.RequestPool = dup.RequestPool
	new.RequestBotID = dup.RequestBotID

	// Other updates derived from dup.
	new.DedupedFrom = dup.TaskRunID()
	new.CostSavedUSD = dup.CostUSD
	serverVersions := stringset.NewFromSlice(dup.ServerVersions...)
	serverVersions.Add(c.ServerVersion)
	new.ServerVersions = serverVersions.ToSlice()
	sort.Strings(new.ServerVersions)

	new.CurrentTaskSlice = int64(curSlice)
	new.TryNumber = datastore.NewIndexedNullable(int64(0))

	// new's Key, RequestName, RequestUser, Tags, Created, Modified remain unchanged.
	// Since dup is a succeeded task, fields for any type of failures are skipped.
	// new is a duplication of dup, so its PropertiesHash should remain empty.
}
