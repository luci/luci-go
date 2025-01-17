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
	"time"

	"github.com/google/uuid"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

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

	// ServerVersion is the version of the executing binary.
	ServerVersion string
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

	// TODO(b/355013435): Dedup by properties hash.
	// TODO(b/355013250): Create ResultDB invocation.
	// TODO(b/355013251): Create TaskToRun.

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
				TaskID:   model.RequestKeyToTaskID(tr.Key, model.AsRequest),
				ExpireAt: now.Add(time.Hour * 24 * 7),
			}
			toPut = append(toPut, tri)
		}

		// TODO(b/355013251): submit to RBE
		// TODO(b/355013510): handle BuildTask
		// TODO(b/355012874): Pubsub notification

		return datastore.Put(ctx, toPut...)
	}, nil)
	if err != nil {
		return nil, errors.Annotate(err, "error saving the task").Err()
	}

	// TODO(b/355013251): report metrics
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
