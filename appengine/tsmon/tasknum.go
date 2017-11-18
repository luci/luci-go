// Copyright 2016 The LUCI Authors.
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

package tsmon

import (
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/info"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/server/tsmon"
)

const (
	instanceNamespace         = "ts_mon_instance_namespace"
	instanceExpirationTimeout = 30 * time.Minute
)

type instance struct {
	_kind       string    `gae:"$kind,Instance"`
	ID          string    `gae:"$id"`
	TaskNum     int       `gae:"task_num"`
	LastUpdated time.Time `gae:"last_updated"`
}

// DatastoreTaskNumAllocator implements TaskNumAllocator on top of datastore.
//
// Its NotifyTaskIsAlive registers a claim for a task number, which is later
// fulfilled by the housekeeping cron.
type DatastoreTaskNumAllocator struct {
}

// NotifyTaskIsAlive is part of TaskNumAllocator interface.
func (DatastoreTaskNumAllocator) NotifyTaskIsAlive(c context.Context, taskID string) (taskNum int, err error) {
	c = info.MustNamespace(c, instanceNamespace)
	err = datastore.RunInTransaction(c, func(c context.Context) error {
		entity := instance{ID: taskID}
		switch err := datastore.Get(c, &entity); {
		case err == datastore.ErrNoSuchEntity:
			entity.TaskNum = -1
		case err != nil:
			return err
		}
		entity.LastUpdated = clock.Now(c).UTC()
		taskNum = entity.TaskNum
		return datastore.Put(c, &entity)
	}, nil)
	if err == nil && taskNum == -1 {
		err = tsmon.ErrNoTaskNumber
	}
	return
}
