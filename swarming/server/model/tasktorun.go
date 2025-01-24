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

package model

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
)

const (
	// TaskToRunShards is the number of TaskToRun entity kinds to shard across.
	TaskToRunShards = 16
)

// TaskToRun defines a TaskRequest slice ready to be scheduled on a bot.
//
// Each TaskRequest results in one or more TaskToRun entities (one per slice).
// They are created sequentially, one by one, as the task progresses through its
// slices. Each TaskToRun is eventually either picked up by a bot for execution
// or expires. Each TaskToRun picked up for execution has an TaskRunResult
// entity (expired ones don't).
//
// A TaskToRun can either be in "native mode" (dispatched via the native
// Swarming scheduler implemented in Python code base) or in "RBE mode"
// (dispatched via the remote RBE scheduler service). This is controlled by
// RBEReservation field.
//
// A TaskToRun (regardless of mode) can be in two states:
//
// 1. "reapable"
//   - Native mode: QueueNumber and Expiration are both set.
//   - RBE mode: ClaimID is unset and Expiration is set.
//
// 2. "consumed":
//   - Native mode: QueueNumber and Expiration are both unset.
//   - RBE mode: ClaimID is set and Expiration is unset.
//
// The entity starts its life in reapable state and then transitions to consumed
// state either by being picked up by a bot for execution or when it expires.
// Consumed state is final.
//
// The key ID is (see TaskToRunID):
// - lower 4 bits is the try number. The only supported value is 1 now.
// - next 5 bits are TaskResultSummary.CurrentTaskSlice (shifted by 4 bits).
// - the rest is 0.
//
// This entity is stored using a bunch of different shards. The shard number is
// derived deterministically by calculating dimensions hash % TaskToRunShards,
// see TaskToRunKey.
type TaskToRun struct {
	// Extra are entity properties that didn't match any declared ones below.
	//
	// Should normally be empty.
	Extra datastore.PropertyMap `gae:"-,extra"`

	// Key identifies the task and its slice, see TaskToRunKey().
	//
	// Note that the kind is TaskToRunShard<index>, see TaskToRunKind().
	Key *datastore.Key `gae:"$key"`

	// Created is used to know when the entity is enqueued.
	//
	// The very first TaskToRun has the same value as TaskRequest.Created, but the
	// following ones (when using multiple task slices) have Created set at the
	// time they are created.
	//
	// Used in both native and RBE mode.
	Created time.Time `gae:"created_ts,noindex"`

	// Dimensions is a copy of dimensions from the corresponding task slice of
	// TaskRequest.
	//
	// It is used to quickly check if a bot can reap this TaskToRun right after
	// fetching it from a datastore query.
	//
	// Used in both native and RBE mode.
	Dimensions TaskDimensions `gae:"dimensions"`

	// RBEReservation is the RBE reservation name that is (or will be) handling
	// this TaskToRun.
	//
	// If set, then TaskToRunShard is in RBE mode. If not, then in native
	// mode. TaskToRunShard in RBE mode are always (transactionally) created with
	// a Task Queue task to actually dispatch them to the RBE scheduler.
	RBEReservation string `gae:"rbe_reservation,noindex"`

	// Expiration is the scheduling deadline for this TaskToRun.
	//
	// It is based on TaskSlice.Expiration. It is used to figure out when to
	// fallback on the next task slice. It is scanned by a cron job and thus needs
	// to be indexed.
	//
	// It is unset when the TaskToRun is claimed, canceled or expires.
	//
	// Used in both native and RBE mode.
	Expiration datastore.Optional[time.Time, datastore.Indexed] `gae:"expiration_ts"`

	// QueueNumber is a magical number by which bots and tasks find one another.
	//
	// Used only in native mode. Always unset and unused in RBE mode.
	//
	// Priority and request creation timestamp are mixed together to allow queries
	// to order the results by this field to allow sorting by priority first, and
	// then timestamp.
	//
	// Gets unset when the TaskToRun is consumed.
	QueueNumber datastore.Optional[int64, datastore.Indexed] `gae:"queue_number"`

	// ClaimID is set if some bot claimed this TaskToRun and will execute it.
	//
	// Used only in RBE mode. Always unset in native mode.
	//
	// It is an opaque ID supplied by the bot when it attempts to claim this
	// entity. If TaskToRun is already claimed and ClaimID matches the one
	// supplied by the bot, then it means this bot has actually claimed the entity
	// already and now just retries the call.
	//
	// Never gets unset once set.
	ClaimID datastore.Optional[string, datastore.Unindexed] `gae:"claim_id"`

	// ExpirationDelay is a delay from Expiration to the actual expiry time.
	//
	// This is set at expiration process if the last task slice expired by
	// reaching its deadline. Unset if the last slice expired because there were
	// no bots that could run it.
	//
	// Exclusively for monitoring.
	ExpirationDelay datastore.Optional[float64, datastore.Unindexed] `gae:"expiration_delay"`
}

// IsReapable returns true if the TaskToRun is still pending.
func (t *TaskToRun) IsReapable() bool {
	return t.Expiration.IsSet()
}

// Consume moves t into non-reapable state (e.g. when canceling).
func (t *TaskToRun) Consume(claimID string) {
	t.ClaimID = datastore.NewUnindexedOptional(claimID)
	t.Expiration = datastore.Optional[time.Time, datastore.Indexed]{}
	t.QueueNumber = datastore.Optional[int64, datastore.Indexed]{}
}

// TaskSliceIndex returns the entity's task slice index.
func (t *TaskToRun) TaskSliceIndex() int {
	return int(t.Key.IntID() >> 4)
}

// ShardIndex returns the entity's TaskToRun shard index.
func (t *TaskToRun) ShardIndex() (int32, error) {
	kind := t.Key.Kind()
	idx, err := strconv.ParseInt(kind[len("TaskToRunShard"):], 10, 32)
	if err != nil {
		return 0, err
	}
	return int32(idx), nil
}

// MustShardIndex is like ShardIndex, but panics when encounters error.
func (t *TaskToRun) MustShardIndex() int32 {
	idx, err := t.ShardIndex()
	if err != nil {
		panic(err)
	}
	return idx
}

// TaskToRunKey builds a TaskToRun key given the task request key, the entity
// kind shard index and the task to run ID.
func TaskToRunKey(ctx context.Context, taskReq *datastore.Key, shardIdx int32, ttrID int64) *datastore.Key {
	return datastore.NewKey(ctx, TaskToRunKind(shardIdx), "", ttrID, taskReq)
}

// TaskToRunKind returns the TaskToRun entity kind name given a shard index.
func TaskToRunKind(shardIdx int32) string {
	return fmt.Sprintf("TaskToRunShard%d", shardIdx)
}

// TaskRequestToToRunKey builds a TaskToRun key given the task request and the
// slice index.
func TaskRequestToToRunKey(ctx context.Context, taskReq *TaskRequest, sliceIndex int) (*datastore.Key, error) {
	if sliceIndex < 0 || sliceIndex >= len(taskReq.TaskSlices) {
		return nil, errors.Reason("sliceIndex %d out of range: [0, %d)", sliceIndex, len(taskReq.TaskSlices)).Err()
	}
	shardIndex := sliceToToRunShardIndex(taskReq.TaskSlices[sliceIndex])
	ttrID := int64(1 | (sliceIndex << 4))
	return TaskToRunKey(ctx, taskReq.Key, shardIndex, ttrID), nil
}

func sliceToToRunShardIndex(slice TaskSlice) int32 {
	hash := slice.Properties.Dimensions.Hash()
	return int32(hash % TaskToRunShards)
}

// NewTaskToRun creates a new TaskToRun entity.
func NewTaskToRun(ctx context.Context, swarmingProject string, tr *TaskRequest, sliceIndex int) (*TaskToRun, error) {
	ttrKey, err := TaskRequestToToRunKey(ctx, tr, sliceIndex)
	if err != nil {
		return nil, err
	}

	created := tr.Created
	if sliceIndex > 0 {
		created = clock.Now(ctx).UTC()
	}

	// TODO(vadimsh): These expiration timestamps may end up significantly larger
	// than expected if slices are skipped quickly without waiting due to
	// "no capacity" condition. For example, if 4 slices with 1h expiration all
	// were skipped quickly, the fifth one ends up with effectively +4h of extra
	// expiration time.
	offset := 0
	for i := 0; i <= sliceIndex; i++ {
		offset += int(tr.TaskSlices[i].ExpirationSecs)
	}
	exp := datastore.NewIndexedOptional(tr.Created.Add(time.Duration(offset) * time.Second))

	ttr := &TaskToRun{
		Key:            ttrKey,
		Created:        created,
		Dimensions:     tr.TaskSlices[sliceIndex].Properties.Dimensions,
		Expiration:     exp,
		RBEReservation: NewReservationID(swarmingProject, tr.Key, sliceIndex),
	}
	return ttr, nil
}

// NewReservationID generates an RBE reservation ID representing a particular slice.
//
// It needs to globally (potentially across Swarming instances) identify
// a particular task slice. Used to idempotently submit RBE reservations.
//
// Placing this function in rbe package would cause an import loop, so putting
// it here instead.
func NewReservationID(swarmingProject string, reqKey *datastore.Key, sliceIndex int) string {
	return fmt.Sprintf("%s-%s-%d", swarmingProject, RequestKeyToTaskID(reqKey, AsRequest), sliceIndex)
}
