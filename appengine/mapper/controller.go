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
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"

	"go.chromium.org/luci/appengine/tq"

	"go.chromium.org/luci/appengine/mapper/internal/tasks"
	"go.chromium.org/luci/appengine/mapper/splitter"
)

// ID identifies a mapper registered in the controller.
//
// It will be passed across processes, so all processes that execute mapper jobs
// should register same mappers under same IDs.
//
// The safest approach is to keep mapper IDs in the app unique, e.g. do NOT
// reuse them when adding new mappers or significantly changing existing ones.
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
	//
	// May be called multiple times for same key (thus should be idempotent).
	//
	// Returning a transient error indicates that the processing of this batch of
	// keys should be retried (even if some keys were processed successfully).
	//
	// Returning a fatal error causes the entire shard (and eventually the entire
	// job) to be marked as failed. The processing of the failed shard stops right
	// away, but other shards are kept running until completion (or their own
	// failure).
	//
	// The function is called outside of any transactions, so it can start its own
	// if needed.
	Process(c context.Context, p Params, keys []*datastore.Key) error
}

// Controller is responsible for starting, progressing and finishing mapping
// jobs.
//
// It should be treated as a global singleton object. Having more than one
// controller in the production application is a bad idea (they'll collide with
// each other since they use global datastore namespace). It's still useful
// to instantiate multiple controllers in unit tests.
type Controller struct {
	// MapperQueue is a name of the GAE task queue to use for mapping jobs.
	//
	// This queue will perform all "heavy" tasks. It should be configured
	// appropriately to allow desired number of shards to run in parallel.
	//
	// For example, if the largest submitted job is expected to have 128 shards,
	// max_concurrent_requests setting of the mapper queue should be at least 128,
	// otherwise some shards will be stalled waiting for others to finish
	// (defeating the purpose of having large number of shards).
	//
	// If empty, "default" is used.
	MapperQueue string

	// ControlQueue is a name of the GAE task queue to use for control signals.
	//
	// This queue is used very lightly when starting and stopping jobs (roughly
	// 2*Shards tasks overall per job). A default queue.yaml settings for such
	// queue should be sufficient (unless you run a lot of different jobs at
	// once).
	//
	// If empty, "default" is used.
	ControlQueue string

	m       sync.RWMutex
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
	ctl.m.Lock()
	defer ctl.m.Unlock()

	if ctl.disp != nil {
		panic("mapper.Controller is already installed into a tq.Dispatcher")
	}
	ctl.disp = disp

	disp.RegisterTask(&tasks.SplitAndLaunch{}, ctl.splitAndLaunchHandler, ctl.ControlQueue, nil)
	disp.RegisterTask(&tasks.FanOutShards{}, ctl.fanOutShardsHandler, ctl.ControlQueue, nil)
	disp.RegisterTask(&tasks.ProcessShard{}, ctl.processShardHandler, ctl.MapperQueue, nil)
}

// tq returns a dispatcher set in Install or panics if not set yet.
//
// Grabs the reader lock inside.
func (ctl *Controller) tq() *tq.Dispatcher {
	ctl.m.RLock()
	defer ctl.m.RUnlock()
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

	ctl.m.Lock()
	defer ctl.m.Unlock()

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
// Grabs the reader lock inside. Can return only fatal errors.
func (ctl *Controller) getMapper(id ID) (Mapper, error) {
	ctl.m.RLock()
	defer ctl.m.RUnlock()
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

////////////////////////////////////////////////////////////////////////////////
// Task queue tasks handlers.

// splitAndLaunchHandler splits the job into shards and enqueues tasks that
// process shards.
func (ctl *Controller) splitAndLaunchHandler(c context.Context, payload proto.Message) error {
	msg := payload.(*tasks.SplitAndLaunch)
	now := clock.Now(c).UTC()

	// Fetch job details. Make sure it isn't canceled and isn't running already.
	job, err := getJobInState(c, JobID(msg.JobId), JobStateStarting)
	if err != nil || job == nil {
		return errors.Annotate(err, "in SplitAndLaunch").Err()
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

	// Create entities that hold shards state. Each one is in its own entity
	// group, since the combined write rate to them is O(ShardCount), which can
	// overcome limits of a single entity group.
	shards := make([]*shard, len(ranges))
	for idx, rng := range ranges {
		shards[idx] = &shard{
			JobID:   job.ID,
			Index:   idx,
			State:   shardStateStarting,
			Range:   rng,
			Created: now,
			Updated: now,
		}
	}

	// We use auto-generated keys for shards to make sure crashed SplitAndLaunch
	// task retries cleanly, even if the underlying key space we are mapping over
	// changes between the retries (making a naive put using "<job-id>:<index>"
	// key non-idempotent!).
	if err := datastore.Put(c, shards); err != nil {
		return errors.Annotate(err, "failed to store shards").Tag(transient.Tag).Err()
	}

	// Prepare shardList which is basically a manual fully consistent index for
	// Job -> [Shard] relation. We can't use a regular index, since shards are all
	// in different entity groups (see O(ShardCount) argument above).
	//
	// Log the resulting shards along the way.
	shardsEnt := shardList{
		Parent: datastore.KeyForObj(c, job),
		Shards: make([]int64, len(shards)),
	}
	for idx, s := range shards {
		shardsEnt.Shards[idx] = s.ID

		l, r := "-inf", "+inf"
		if s.Range.Start != nil {
			l = s.Range.Start.String()
		}
		if s.Range.End != nil {
			r = s.Range.End.String()
		}
		logging.Infof(c, "Shard #%d is %d: %s - %s", idx, s.ID, l, r)
	}

	// Transactionally associate shards with the job and launch the TQ task that
	// kicks off the processing of each individual shard. We use an intermediary
	// task for this since transactionally launching O(ShardCount) tasks hits TQ
	// transaction limits.
	//
	// If SplitAndLaunch crashes before this transaction lands, there'll be some
	// orphaned Shard entities, no big deal.
	return runTxn(c, func(c context.Context) error {
		job, err := getJobInState(c, JobID(msg.JobId), JobStateStarting)
		if err != nil || job == nil {
			return errors.Annotate(err, "in SplitAndLaunch txn").Err()
		}

		job.State = JobStateRunning
		job.Updated = now
		if err := datastore.Put(c, job, &shardsEnt); err != nil {
			return errors.Annotate(err,
				"when storing Job %d and ShardList with %d shards", job.ID, len(shards),
			).Tag(transient.Tag).Err()
		}

		return ctl.tq().AddTask(c, &tq.Task{
			Title: fmt.Sprintf("job-%d", job.ID),
			Payload: &tasks.FanOutShards{
				JobId: int64(job.ID),
			},
		})
	})
}

// fanOutShardsHandler fetches a list of shards from the job and launches
// named ProcessShard tasks, one per shard.
func (ctl *Controller) fanOutShardsHandler(c context.Context, payload proto.Message) error {
	msg := payload.(*tasks.FanOutShards)

	// Make sure the job isn't canceled yet.
	job, err := getJobInState(c, JobID(msg.JobId), JobStateRunning)
	if err != nil || job == nil {
		return errors.Annotate(err, "in FanOutShards").Err()
	}

	// Grab the list of shards created in SplitAndLaunch. It must exist at this
	// point, since the job is in Running state.
	shardIDs, err := job.fetchShardIDs(c)
	if err != nil {
		return errors.Annotate(err, "in FanOutShards").Err()
	}

	// Enqueue a bunch of named ProcessShard tasks (one per shard) to actually
	// launch shard processing. This is idempotent operation, so if FanOutShards
	// crashes midway and later retried, nothing bad happens.
	tsks := make([]*tq.Task, len(shardIDs))
	for idx, sid := range shardIDs {
		tsks[idx] = makeProcessShardTask(job.ID, sid, 0, true)
	}
	return ctl.tq().AddTask(c, tsks...)
}

// processShardHandler reads a bunch of entities (up to PageSize), and hands
// them to the mapper.
//
// After doing this in a loop for 1 min, it checkpoints the state and reenqueues
// itself to resume mapping in another instance of the task. This makes each
// processing TQ task relatively small, so it doesn't eat a lot of memory, or
// produces gigantic unreadable logs. It also makes TQ's "Pause queue" button
// more handy.
func (ctl *Controller) processShardHandler(c context.Context, payload proto.Message) error {
	msg := payload.(*tasks.ProcessShard)

	// Grab the shard. This returns (nil, nil) if this Task Queue task is stale
	// (based on taskNum) and should be silently skipped.
	sh, err := getActiveShard(c, msg.ShardId, msg.TaskNum)
	if err != nil || sh == nil {
		return errors.Annotate(err, "when fetching shard state").Err()
	}
	c = withShardIndex(c, sh.Index)

	logging.Infof(c,
		"Resuming processing of the shard (launched %s ago)",
		clock.Now(c).Sub(sh.Created))

	// Grab the job config, make sure the job is still active.
	job, err := getJobInState(c, JobID(msg.JobId), JobStateRunning)
	if err != nil || job == nil {
		return errors.Annotate(err, "in ProcessShard").Err()
	}

	// Fail the shard if the mapper is suddenly gone.
	mapper, err := ctl.getMapper(job.Config.Mapper)
	if err != nil {
		return ctl.finishShard(c, sh.ID, err)
	}

	baseQ := job.Config.Query.ToDatastoreQuery()
	lastKey := sh.ResumeFrom
	keys := make([]*datastore.Key, 0, job.Config.PageSize)

	shardDone := false // true when finished processing the shard
	pageCount := 0     // how many pages processed successfully

	// A soft deadline when to checkpoint the progress and reenqueue the
	// processing task. We never abort processing of a page midway (causes too
	// many complications), so if the mapper is extremely slow, it may end up
	// running longer than this deadline.
	dur := time.Minute
	if job.Config.TaskDuration > 0 {
		dur = job.Config.TaskDuration
	}
	deadline := clock.Now(c).Add(dur)

	// Optionally also put a limit on number of processed pages. Useful if the
	// mapper is somehow leaking resources (not sure it is possible in Go, but
	// it was definitely possible in Python).
	pageCountLimit := math.MaxInt32
	if job.Config.PagesPerTask > 0 {
		pageCountLimit = job.Config.PagesPerTask
	}

	for clock.Now(c).Before(deadline) && pageCount < pageCountLimit {
		rng := sh.Range
		if lastKey != nil {
			rng.Start = lastKey
		}
		if rng.IsEmpty() {
			shardDone = true
			break
		}

		// Fetch next batch of keys. Return an error to the outer scope where it
		// eventually will bubble up to TQ (so the task is retried with exponential
		// backoff).
		logging.Infof(c, "Fetching the next batch...")
		q := rng.Apply(baseQ).Limit(int32(job.Config.PageSize)).KeysOnly(true)
		keys = keys[:0]
		if err = datastore.GetAll(c, q, &keys); err != nil {
			err = errors.Annotate(err, "when querying for keys").Tag(transient.Tag).Err()
			break
		}

		// No results within the range? Processing of the shard is complete!
		if len(keys) == 0 {
			shardDone = true
			break
		}

		// Let the mapper do its thing. Remember where to resume from.
		logging.Infof(c,
			"Processing %d entities: %s - %s",
			len(keys),
			keys[0].String(),
			keys[len(keys)-1].String())
		if err = mapper.Process(c, job.Config.Params, keys); err != nil {
			err = errors.Annotate(err, "in the mapper %q", mapper.MapperID()).Err()
			break
		}
		lastKey = keys[len(keys)-1]
		pageCount++

		// Note: at this point we might try to checkpoint the progress, but we must
		// be careful not to exceed 1 transaction per second limit. Considering we
		// also MUST checkpoint the progress at the end of the task, it is a bit
		// tricky to guarantee no two checkpoints are closer than 1 sec. We can do
		// silly things like sleep 1 sec before the last checkpoint, but they
		// provide no guarantees.
		//
		// So instead we store the progress after the deadline is up. If the task
		// crashes midway, up to 1 min of work will be retried. No big deal.
	}

	// We are done with the shard when either processed all its range or failed
	// with a fatal error. finishShard would take care of notifying the parent
	// job about the shard's completion.
	if shardDone || (err != nil && !transient.Tag.In(err)) {
		return ctl.finishShard(c, sh.ID, err)
	}

	logging.Infof(c, "The shard processing will resume from %s", lastKey)

	// If the shard isn't done and we made no progress at all, then we hit
	// a transient error. Ask TQ to retry.
	if pageCount == 0 {
		return err
	}

	// Otherwise need to checkpoint the progress and either to retry this task
	// (on transient errors, to get an exponential backoff from TQ), or start
	// a new task.
	txnErr := shardTxn(c, sh.ID, func(c context.Context, sh *shard) (bool, error) {
		switch {
		case sh.ProcessTaskNum != msg.TaskNum:
			logging.Warningf(c, "Unexpected shard state: its ProcessTaskNum is %d != %d", sh.ProcessTaskNum, msg.TaskNum)
			return false, nil // some other task is already running
		case sh.ResumeFrom != nil && lastKey.Less(sh.ResumeFrom):
			logging.Warningf(c, "Unexpected shard state: its ResumeFrom is %s >= %s", sh.ResumeFrom, lastKey)
			return false, nil // someone already claimed to process further, let them proceed
		}

		sh.State = shardStateRunning
		sh.ResumeFrom = lastKey

		// If the processing failed, just store the progress, but do not start a
		// new TQ task. Retry the current task instead (to get exponential backoff).
		if err != nil {
			return true, nil
		}

		// Otherwise launch a new task in the chain. This essentially "resets"
		// the exponential backoff counter.
		sh.ProcessTaskNum++
		return true, ctl.tq().AddTask(c,
			makeProcessShardTask(sh.JobID, sh.ID, sh.ProcessTaskNum, false))
	})

	switch {
	case err != nil && txnErr == nil:
		return err
	case err == nil && txnErr != nil:
		return errors.Annotate(txnErr, "when storing shard progress").Err()
	case err != nil && txnErr != nil:
		return errors.Annotate(txnErr, "when storing shard progress after a transient error (%s)", err).Err()
	default: // (nil, nil)
		return nil
	}
}

// finishShard marks the shard as finished (with status based on shardErr) and
// emits a task to update the parent job's status.
func (ctl *Controller) finishShard(c context.Context, shardID int64, shardErr error) error {
	err := shardTxn(c, shardID, func(c context.Context, sh *shard) (save bool, err error) {
		// TODO: notify the parent job about shard's completion.
		runtime := clock.Now(c).Sub(sh.Created)
		if shardErr != nil {
			logging.Errorf(c, "The shard processing failed in %s with error: %s", runtime, shardErr)
			sh.State = shardStateFail
			sh.Error = shardErr.Error()
		} else {
			logging.Infof(c, "The shard processing finished successfully in %s", runtime)
			sh.State = shardStateSuccess
		}
		return true, nil
	})
	return errors.Annotate(err, "when marking the shard as finished").Err()
}

var shardIndexKey = "mapper.ShardIndexForTest"

// withShardIndex puts the shard index into the context for logging and tests.
func withShardIndex(c context.Context, idx int) context.Context {
	c = logging.SetField(c, "shardIdx", idx)
	c = context.WithValue(c, &shardIndexKey, idx)
	return c
}

// makeProcessShardTask creates a ProcessShard tq.Task.
//
// If 'named' is true, assigns it a name. Tasks are named based on their shard
// IDs and an index in the chain of ProcessShard tasks (task number), so that
// on retries we don't rekick already finished tasks.
func makeProcessShardTask(job JobID, shardID, taskNum int64, named bool) *tq.Task {
	// Note: strictly speaking including job ID in the task name is redundant,
	// since shardID is already globally unique, but it doesn't hurt. Useful for
	// debugging and when looking at logs and pending TQ tasks.
	t := &tq.Task{
		Title: fmt.Sprintf("job-%d-shard-%d-task-%d", job, shardID, taskNum),
		Payload: &tasks.ProcessShard{
			JobId:   int64(job),
			ShardId: shardID,
			TaskNum: taskNum,
		},
	}
	if named {
		t.DeduplicationKey = fmt.Sprintf("v1-%d-%d-%d", job, shardID, taskNum)
	}
	return t
}
