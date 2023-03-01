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
	"fmt"
	"time"

	"go.chromium.org/luci/gae/service/datastore"
)

// TaskToRun defines a TaskRequest slice ready to be scheduled on a bot.
type TaskToRun struct {
	// Kind is TaskToRunShard<index>.
	Kind string `gae:"$kind"`
	// ID is derived from the slice index.
	ID int64 `gae:"$id"`
	// Parent is the parent TaskRequest key.
	Parent *datastore.Key `gae:"$parent"`

	// Expiration is the scheduling deadline for this TaskToRun.
	//
	// It is unset if the TaskToRun has already been claimed, was canceled or
	// has expired.
	Expiration time.Time `gae:"expiration_ts"`

	_extra datastore.PropertyMap `gae:"-,extra"`
}

// IsReapable returns true if the TaskToRun is still pending.
func (t *TaskToRun) IsReapable() bool {
	return !t.Expiration.IsZero()
}

// TaskToRunKind returns the TaskToRun entity kind name given a shard index.
func TaskToRunKind(shardIdx int32) string {
	return fmt.Sprintf("TaskToRunShard%d", shardIdx)
}
