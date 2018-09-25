// Copyright 2018 The LUCI Authors.
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

package mapper

import (
	"fmt"
	"sync"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"

	"go.chromium.org/luci/appengine/tq"

	"go.chromium.org/luci/appengine/mapper/internal/tasks"
	"go.chromium.org/luci/appengine/mapper/splitter"
)

// ID identifies a mapper.
type ID string

// Mapper knows how to apply some function to a slice of entities.
//
// Mappers are supplied by the users of the library and registered in the
// controller via RegisterMapper call.
type Mapper interface {
	// MapperID uniquely identifies this mapper.
	//
	// It will be used as identifier of the mapper across process boundaries.
	MapperID() ID

	// Process applies some function to the given slice of entities, given by
	// their keys.
	Process(c context.Context, p Params, keys []*datastore.Key) error
}

// Controller is responsible for starting, progressing and finishing mapping
// jobs.
//
// It should be treated as a global singleton object. There's rarely a need to
// have more than one controller per application.
type Controller struct {
	// MapperQueue is a name of the GAE task queue to use for mapping jobs.
	//
	// This queue will perform all "heavy" tasks. It should be configured
	// appropriately to allow desired number of tasks to run in parallel.
	//
	// If empty, "default" is used.
	MapperQueue string

	// ControlQueue is a name of the GAE task queue to use for control signals.
	//
	// This queue is used very lightly when starting and stopping jobs.
	//
	// If empty, "default" is used.
	ControlQueue string

	l       sync.RWMutex
	mappers map[ID]Mapper
	disp    *tq.Dispatcher

	// Called in splitAndLaunchHandler during tests.
	testingRecordSplit func(rng splitter.Range)
}

// Install registers task queue task handlers in the given task queue
// dispatcher.
//
// This must be done before Controller is used.
//
// There can be at most one Controller installed into an instance of TQ
// dispatcher. Installing more will cause panics.
//
// If you need multiple different controllers for some reason, create multiple
// tq.Dispatchers (with different base URLs, so they don't conflict with each
// other) and install them all into the router.
func (ctl *Controller) Install(disp *tq.Dispatcher) {
	ctl.l.Lock()
	defer ctl.l.Unlock()

	if ctl.disp != nil {
		panic("mapper.Controller is already installed into a tq.Dispatcher")
	}
	ctl.disp = disp

	disp.RegisterTask(&tasks.SplitAndLaunch{}, ctl.splitAndLaunchHandler, ctl.ControlQueue, nil)
}

// tq returns a dispatcher set in Install or panics if not set yet.
//
// Grabs the reader lock inside.
func (ctl *Controller) tq() *tq.Dispatcher {
	ctl.l.RLock()
	defer ctl.l.RUnlock()
	if ctl.disp == nil {
		panic("mapper.Controller wasn't installed into tq.Dispatcher yet")
	}
	return ctl.disp
}

// RegisterMapper adds the given mapper to the internal registry.
//
// Intended to be used during init() time or early during the process
// initialization. Panics if a mapper with such ID has already been registered.
//
// The mapper ID will be used internally to identify which mapper a job should
// be using. If a mapper disappears while the job is running (e.g. if the
// service binary is updated and new binary doesn't have the mapper registered
// anymore), the job ends with a failure.
func (ctl *Controller) RegisterMapper(m Mapper) {
	id := m.MapperID()

	ctl.l.Lock()
	defer ctl.l.Unlock()

	if _, ok := ctl.mappers[id]; ok {
		panic(fmt.Sprintf("mapper %q is already registered", id))
	}

	if ctl.mappers == nil {
		ctl.mappers = make(map[ID]Mapper, 1)
	}
	ctl.mappers[id] = m
}

// getMapper returns a registered mapper or an error.
//
// Grabs the reader lock inside.
func (ctl *Controller) getMapper(id ID) (Mapper, error) {
	ctl.l.RLock()
	defer ctl.l.RUnlock()
	if m, ok := ctl.mappers[id]; ok {
		return m, nil
	}
	return nil, errors.Reason("no mapper with ID %q registered", id).Err()
}

// LaunchJob launches a new mapping job, returning its ID (that can be used to
// control it or query its status).
//
// Launches a datastore transaction inside.
func (ctl *Controller) LaunchJob(c context.Context, j *JobConfig) (JobID, error) {
	disp := ctl.tq()

	if err := j.Validate(); err != nil {
		return 0, errors.Annotate(err, "bad job config").Err()
	}
	if _, err := ctl.getMapper(j.Mapper); err != nil {
		return 0, errors.Annotate(err, "bad job config").Err()
	}

	// Prepare and store the job entity, generate its key. Launch a tq task that
	// subdivides the key space and launches individual shards. We do it
	// asynchronously since this can be potentially slow (for large number of
	// shards).
	var job Job
	err := runTxn(c, func(c context.Context) error {
		now := clock.Now(c).UTC()
		job = Job{
			Config:  *j,
			State:   JobStateStarting,
			Created: now,
			Updated: now,
		}
		if err := datastore.Put(c, &job); err != nil {
			return errors.Annotate(err, "failed to store Job entity").Tag(transient.Tag).Err()
		}
		return disp.AddTask(c, &tq.Task{
			Title: fmt.Sprintf("job-%d", job.ID),
			Payload: &tasks.SplitAndLaunch{
				JobId: int64(job.ID),
			},
		})
	})
	if err != nil {
		return 0, err
	}
	return job.ID, nil
}

// getJob fetches a Job entity.
//
// Recognizes and tags transient errors.
func (ctl *Controller) getJob(c context.Context, id JobID) (*Job, error) {
	job := &Job{ID: id}
	switch err := datastore.Get(c, job); {
	case err == datastore.ErrNoSuchEntity:
		return nil, errors.Reason("no mapping job with ID %d", id).Err()
	case err != nil:
		return nil, errors.Annotate(err, "failed to fetch Job entity with ID %d", id).Tag(transient.Tag).Err()
	default:
		return job, nil
	}
}

////////////////////////////////////////////////////////////////////////////////
// Task queue tasks handlers.

// splitAndLaunchHandler splits the job into shards and enqueues tasks that
// process shards.
func (ctl *Controller) splitAndLaunchHandler(c context.Context, payload proto.Message) error {
	msg := payload.(*tasks.SplitAndLaunch)

	// Fetch job details. Make sure it isn't canceled and isn't running already.
	job, err := ctl.getJob(c, JobID(msg.JobId))
	switch {
	case err != nil:
		return errors.Annotate(err, "in SplitAndLaunch").Err()
	case job.State != JobStateStarting:
		logging.Warningf(c, "Skipping the job: its state is %s, expecting %s", job.State, JobStateStarting)
		return nil
	}

	// Figure out key ranges for shards. There may be fewer shards than requested
	// if there are too few entities.
	dq := job.Config.Query.ToDatastoreQuery()
	ranges, err := splitter.SplitIntoRanges(c, dq, splitter.Params{
		Shards:  job.Config.ShardCount,
		Samples: 512, // should be enough for everyone...
	})
	if err != nil {
		return errors.Annotate(err, "failed to split the query into shards").Tag(transient.Tag).Err()
	}

	// Log the resulting shards.
	for idx, rng := range ranges {
		l, r := "-inf", "+inf"
		if rng.Start != nil {
			l = rng.Start.String()
		}
		if rng.End != nil {
			r = rng.End.String()
		}
		logging.Infof(c, "Shard %d: %s - %s", idx, l, r)
		if ctl.testingRecordSplit != nil {
			ctl.testingRecordSplit(rng)
		}
	}

	// TODO(vadimsh): Do the rest of the magic:
	// 1. Create a bunch of Shard entities, each with assigned range.
	// 2. Transactionally assign shards to Job and launch a task that kicks off
	//    processing of shards.
	return nil
}
