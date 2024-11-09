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
	"context"
	"fmt"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/gae/filter/dscache"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/server/tsmon"
)

// DatastoreNamespace is a datastore namespace with all tsmon state.
const DatastoreNamespace = "ts_mon_instance_namespace"

// DatastoreTaskNumAllocator implements TaskNumAllocator on top of datastore.
//
// Its NotifyTaskIsAlive registers a claim for a task number, which is later
// fulfilled by the housekeeping cron (see AssignTaskNumbers).
type DatastoreTaskNumAllocator struct {
}

// NotifyTaskIsAlive is part of TaskNumAllocator interface.
func (DatastoreTaskNumAllocator) NotifyTaskIsAlive(ctx context.Context, task *target.Task, instanceID string) (taskNum int, err error) {
	ctx = dsContext(ctx)

	// Exact values here are not important. Important properties are:
	//  * 'entityID' is unique, and depends on both 'task' and 'instanceID'.
	//  * All instances from same task have same 'target' value.
	//
	// The cron ('AssignTaskNumbers') will fetch all entities and will group them
	// by 'target' value before assigning task numbers.
	target := fmt.Sprintf("%s|%s|%s|%s", task.DataCenter, task.ServiceName, task.JobName, task.HostName)
	entityID := fmt.Sprintf("%s|%s", target, instanceID)

	err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		entity := instance{ID: entityID}
		switch err := datastore.Get(ctx, &entity); {
		case err == datastore.ErrNoSuchEntity:
			entity.Target = target
			entity.TaskNum = -1
		case err != nil:
			return err
		}
		entity.LastUpdated = clock.Now(ctx).UTC()
		taskNum = entity.TaskNum
		return datastore.Put(ctx, &entity)
	}, nil)
	if err == nil && taskNum == -1 {
		err = tsmon.ErrNoTaskNumber
	}
	return
}

// AssignTaskNumbers updates the set of task number requests created with
// DatastoreTaskNumAllocator.
//
// It assigns unique task numbers to those without ones set, and expires old
// ones (thus reclaiming task numbers assigned to them).
//
// Must be used from some (global per project) cron if DatastoreTaskNumAllocator
// is used. Use 'InstallHandlers' to install the corresponding cron handler.
func AssignTaskNumbers(ctx context.Context) error {
	ctx = dsContext(ctx)

	now := clock.Now(ctx)
	cutoff := now.Add(-instanceExpirationTimeout)
	perTarget := map[string]*workingSet{}

	// Enumerate all instances stored in the datastore and expire old ones (in
	// batches). Collect a set of used task numbers and a set of instances not
	// assigned a number yet. Group by 'Target' (can be "" for old entities, this
	// is fine).
	q := datastore.NewQuery("Instance")
	err := datastore.RunBatch(ctx, int32(taskQueryBatchSize), q, func(entity *instance) error {
		set := perTarget[entity.Target]
		if set == nil {
			set = newWorkingSet(cutoff)
			perTarget[entity.Target] = set
		}
		set.addInstance(ctx, entity)
		if len(set.expired) >= taskQueryBatchSize {
			if err := set.cleanupExpired(ctx); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return errors.Annotate(err, "failed to enumerate or expire entries").Err()
	}

	return parallel.FanOutIn(func(tasks chan<- func() error) {
		for target, set := range perTarget {
			target := target
			set := set

			tasks <- func() error {
				// "Flush" all pending expired instances.
				if err := set.cleanupExpired(ctx); err != nil {
					logging.WithError(err).Errorf(ctx, "Failed to delete expired entries for target %q", target)
					return err
				}
				// Assign task numbers to those that don't have one assigned yet.
				logging.Debugf(ctx, "Found %d expired and %d unassigned instances for target %q", set.totalExpired, len(set.pending), target)
				if err := set.assignTaskNumbers(ctx); err != nil {
					logging.WithError(err).Errorf(ctx, "Failed to assign task numbers for target %q", target)
					return err
				}
				return nil
			}
		}
	})
}

////////////////////////////////////////////////////////////////////////////////

const (
	instanceExpirationTimeout = 30 * time.Minute
	taskQueryBatchSize        = 500
)

// dsContext is used for all datastore accesses that touch 'instance' entities.
//
// It switches the namespace and disables dscache, since these entities are
// updated from Flex, which doesn't work with dscache. Besides, all reads happen
// either in transactions or through queries - dscache is useless anyhow.
func dsContext(ctx context.Context) context.Context {
	ctx = info.MustNamespace(ctx, DatastoreNamespace)
	return dscache.AddShardFunctions(ctx, func(*datastore.Key) (shards int, ok bool) {
		return 0, true
	})
}

// instance corresponds to one process that flushes metrics.
type instance struct {
	_kind  string                `gae:"$kind,Instance"`
	_extra datastore.PropertyMap `gae:"-,extra"`

	ID          string    `gae:"$id"`
	Target      string    `gae:"target,noindex"`
	TaskNum     int       `gae:"task_num,noindex"`
	LastUpdated time.Time `gae:"last_updated,noindex"`

	// Disable dscache to allow these entities be updated from Flex, which doesn't
	// work with dscache. Besides, we update these entities from transactions or
	// based on queries - dscache is useless anywhere.
	_ datastore.Toggle `gae:"$dscache.enable,false"`
}

// workingSet is used internally by AssignTaskNumbers.
type workingSet struct {
	cutoff       time.Time        // if LastUpdate is before => instance has expired
	expired      []*datastore.Key // entities with LastUpdate too long ago
	pending      []*instance      // entities with TaskNum is still -1
	assignedNums map[int]struct{} // assigned already task numbers

	totalExpired int // total number of entities deleted
}

func newWorkingSet(cutoff time.Time) *workingSet {
	return &workingSet{
		cutoff:       cutoff,
		assignedNums: map[int]struct{}{},
	}
}

func (s *workingSet) addInstance(ctx context.Context, entity *instance) {
	switch {
	case entity.LastUpdated.Before(s.cutoff):
		logging.Debugf(ctx, "Expiring %q (task_num %d), inactive since %s",
			entity.ID, entity.TaskNum, entity.LastUpdated)
		s.expired = append(s.expired, datastore.KeyForObj(ctx, entity))
	case entity.TaskNum < 0:
		s.pending = append(s.pending, entity)
	default:
		s.assignedNums[entity.TaskNum] = struct{}{}
	}
}

func (s *workingSet) cleanupExpired(ctx context.Context) error {
	if len(s.expired) == 0 {
		return nil
	}

	logging.Debugf(ctx, "Expiring %d instance(s)", len(s.expired))
	if err := datastore.Delete(ctx, s.expired); err != nil {
		return err
	}

	s.totalExpired += len(s.expired)
	s.expired = s.expired[:0]
	return nil
}

func (s *workingSet) assignTaskNumbers(ctx context.Context) error {
	if len(s.pending) == 0 {
		return nil
	}

	nextNum := gapFinder(s.assignedNums)
	for _, entity := range s.pending {
		entity.TaskNum = nextNum()
		logging.Debugf(ctx, "Assigned %q task_num %d", entity.ID, entity.TaskNum)
	}

	// Update all pending entities. This is non-transactional, meaning:
	//  * We may override newer LastUpdated - no big deal.
	//  * If there are two parallel 'AssignTaskNumbers', they'll screw up each
	//    other. GAE cron gives some protection against concurrent cron job
	//    executions though.
	return datastore.Put(ctx, s.pending)
}

func gapFinder(used map[int]struct{}) func() int {
	next := 0
	return func() int {
		for {
			n := next
			next++
			_, has := used[n]
			if !has {
				return n
			}
		}
	}
}
