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
	"go.chromium.org/luci/common/sync/parallel"

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
	disp.RegisterTask(&tasks.RequestJobStateUpdate{}, ctl.requestJobStateUpdateHandler, ctl.ControlQueue, nil)
	disp.RegisterTask(&tasks.UpdateJobState{}, ctl.updateJobStateHandler, ctl.ControlQueue, nil)
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
			State:   State_STARTING,
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

// GetJob fetches a previously launched job given its ID.
//
// Returns ErrNoSuchJob if not found. All other possible errors are transient
// and they are marked as such.
func (ctl *Controller) GetJob(c context.Context, id JobID) (*Job, error) {
	// Even though we could have made getJob public, we want to force API users
	// to use Controller as a single facade.
	return getJob(c, id)
}

// AbortJob aborts a job and returns its most recent state.
//
// Silently does nothing if the job is finished or already aborted.
//
// Returns ErrNoSuchJob is there's no such job at all. All other possible errors
// are transient and they are marked as such.
func (ctl *Controller) AbortJob(c context.Context, id JobID) (job *Job, err error) {
	err = runTxn(c, func(c context.Context) error {
		var err error
		switch job, err = getJob(c, id); {
		case err != nil:
			return err
		case isFinalState(job.State) || job.State == State_ABORTING:
			return nil // nothing to abort, already done
		case job.State == State_STARTING:
			// Shards haven't been launched yet. Kill the job right away.
			job.State = State_ABORTED
		case job.State == State_RUNNING:
			// Running shards will discover that the job is aborting and will
			// eventually move into ABORTED state (notifying the job about it). Once
			// all shards report they are done, the job itself will switch into
			// ABORTED state.
			job.State = State_ABORTING
		}
		job.Updated = clock.Now(c).UTC()
		return errors.Annotate(datastore.Put(c, job), "failed to store Job entity").Tag(transient.Tag).Err()
	})
	if err != nil {
		job = nil // don't return bogus data in case txn failed to land
	}
	return
}

////////////////////////////////////////////////////////////////////////////////
// Task queue tasks handlers.

// errJobAborted is used internally as shard failure status when the job is
// being aborted.
//
// It causes the shard to switch into ABORTED state instead of FAIL.
var errJobAborted = errors.New("the job has been aborted")

// splitAndLaunchHandler splits the job into shards and enqueues tasks that
// process shards.
func (ctl *Controller) splitAndLaunchHandler(c context.Context, payload proto.Message) error {
	msg := payload.(*tasks.SplitAndLaunch)
	now := clock.Now(c).UTC()

	// Fetch job details. Make sure it isn't canceled and isn't running already.
	job, err := getJobInState(c, JobID(msg.JobId), State_STARTING)
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
			JobID:         job.ID,
			Index:         idx,
			State:         State_STARTING,
			Range:         rng,
			ExpectedCount: -1,
			Created:       now,
			Updated:       now,
		}
	}

	// Calculate number of entities in each shard to track shard processing
	// progress. Note that this can be very slow if there are many entities.
	if job.Config.TrackProgress {
		logging.Infof(c, "Estimating the size of each shard...")
		if err := fetchShardSizes(c, dq, shards); err != nil {
			return errors.Annotate(err, "when estimating shard sizes").Err()
		}
	}

	// We use auto-generated keys for shards to make sure crashed SplitAndLaunch
	// task retries cleanly, even if the underlying key space we are mapping over
	// changes between the retries (making a naive put using "<job-id>:<index>"
	// key non-idempotent!).
	logging.Infof(c, "Instantiating shards...")
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
		count := ""
		if s.ExpectedCount != 0 {
			count = fmt.Sprintf(" (%d entities)", s.ExpectedCount)
		}
		logging.Infof(c, "Shard #%d is %d: %s - %s%s", idx, s.ID, l, r, count)
	}

	// Transactionally associate shards with the job and launch the TQ task that
	// kicks off the processing of each individual shard. We use an intermediary
	// task for this since transactionally launching O(ShardCount) tasks hits TQ
	// transaction limits.
	//
	// If SplitAndLaunch crashes before this transaction lands, there'll be some
	// orphaned Shard entities, no big deal.
	logging.Infof(c, "Updating the job and launching the fan out task...")
	return runTxn(c, func(c context.Context) error {
		job, err := getJobInState(c, JobID(msg.JobId), State_STARTING)
		if err != nil || job == nil {
			return errors.Annotate(err, "in SplitAndLaunch txn").Err()
		}

		job.State = State_RUNNING
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

// fetchShardSizes makes a bunch of Count() queries to figure out size of each
// shard.
//
// Updates ExpectedCount in-place.
func fetchShardSizes(c context.Context, baseQ *datastore.Query, shards []*shard) error {
	err := parallel.WorkPool(32, func(tasks chan<- func() error) {
		for _, sh := range shards {
			sh := sh
			tasks <- func() error {
				n, err := datastore.CountBatch(c, 1024, sh.Range.Apply(baseQ))
				if err == nil {
					sh.ExpectedCount = n
				}
				return errors.Annotate(err, "for shard #%d", sh.Index).Err()
			}
		}
	})
	return transient.Tag.Apply(err)
}

// fanOutShardsHandler fetches a list of shards from the job and launches
// named ProcessShard tasks, one per shard.
func (ctl *Controller) fanOutShardsHandler(c context.Context, payload proto.Message) error {
	msg := payload.(*tasks.FanOutShards)

	// Make sure the job is still present. If it is aborted, we still need to
	// launch the shards, so they notice they are being aborted. We could try
	// to abort all shards right here and now, but it basically means implementing
	// an alternative shard abort flow. Seems simpler just to let the regular flow
	// to proceed.
	job, err := getJobInState(c, JobID(msg.JobId), State_RUNNING, State_ABORTING)
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
	job, err := getJobInState(c, JobID(msg.JobId), State_RUNNING, State_ABORTING)
	if err != nil || job == nil {
		return errors.Annotate(err, "in ProcessShard").Err()
	}

	// If the job is being killed, kill the shard as well. This will eventually
	// notify the job about shard's completion. Once all shards are done, the
	// job will switch into ABORTED state.
	if job.State == State_ABORTING {
		return ctl.finishShard(c, sh.ID, 0, errJobAborted)
	}

	// Fail the shard if the mapper is suddenly gone.
	mapper, err := ctl.getMapper(job.Config.Mapper)
	if err != nil {
		return ctl.finishShard(c, sh.ID, 0, err)
	}

	baseQ := job.Config.Query.ToDatastoreQuery()
	lastKey := sh.ResumeFrom
	keys := make([]*datastore.Key, 0, job.Config.PageSize)

	shardDone := false    // true when finished processing the shard
	pageCount := 0        // how many pages processed successfully
	itemCount := int64(0) // how many entities processed successfully

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
		itemCount += int64(len(keys))

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
		return ctl.finishShard(c, sh.ID, itemCount, err)
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

		sh.State = State_RUNNING
		sh.ResumeFrom = lastKey
		sh.ProcessedCount += itemCount

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
func (ctl *Controller) finishShard(c context.Context, shardID, processedCount int64, shardErr error) error {
	err := shardTxn(c, shardID, func(c context.Context, sh *shard) (save bool, err error) {
		runtime := clock.Now(c).Sub(sh.Created)
		switch {
		case shardErr == errJobAborted:
			logging.Warningf(c, "The job has been aborted, aborting the shard after it has been running %s", runtime)
			sh.State = State_ABORTED
			sh.Error = errJobAborted.Error()
		case shardErr != nil:
			logging.Errorf(c, "The shard processing failed in %s with error: %s", runtime, shardErr)
			sh.State = State_FAIL
			sh.Error = shardErr.Error()
		default:
			logging.Infof(c, "The shard processing finished successfully in %s", runtime)
			sh.State = State_SUCCESS
		}
		sh.ProcessedCount += processedCount
		return true, ctl.requestJobStateUpdate(c, sh.JobID, sh.ID)
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

// requestJobStateUpdate submits RequestJobStateUpdate task, which eventually
// causes updateJobStateHandler to execute.
func (ctl *Controller) requestJobStateUpdate(c context.Context, jobID JobID, shardID int64) error {
	return ctl.tq().AddTask(c, &tq.Task{
		Title: fmt.Sprintf("job-%d-shard-%d", jobID, shardID),
		Payload: &tasks.RequestJobStateUpdate{
			JobId:   int64(jobID),
			ShardId: shardID,
		},
	})
}

// requestJobStateUpdateHandler is called whenever state of some shard changes.
//
// It forwards this notification to the job (specifically updateJobStateHandler)
// throttling the rate to ~0.5 QPS to avoid overwhelming job's entity group with
// high write rate.
func (ctl *Controller) requestJobStateUpdateHandler(c context.Context, payload proto.Message) error {
	msg := payload.(*tasks.RequestJobStateUpdate)

	// Throttle to once per 2 sec (and make sure it is always in the future). We
	// rely here on a pretty good (< .5s maximum skew) clock sync on GAE.
	eta := clock.Now(c).Unix()
	eta = (eta/2 + 1) * 2
	dedupKey := fmt.Sprintf("update-job-state-v1:%d:%d", msg.JobId, eta)

	err := ctl.tq().AddTask(c, &tq.Task{
		DeduplicationKey: dedupKey,
		Title:            fmt.Sprintf("job-%d", msg.JobId),
		ETA:              time.Unix(eta, 0),
		Payload:          &tasks.UpdateJobState{JobId: msg.JobId},
	})
	return errors.Annotate(err, "when adding UpdateJobState task").Err()
}

// updateJobStateHandler is called some time later after one or more shards have
// changed state.
//
// It calculates overall job state based on the state of its shards.
func (ctl *Controller) updateJobStateHandler(c context.Context, payload proto.Message) error {
	msg := payload.(*tasks.UpdateJobState)

	// Get the job and all its shards in their most recent state.
	job, err := getJobInState(c, JobID(msg.JobId), State_RUNNING, State_ABORTING)
	if err != nil || job == nil {
		return errors.Annotate(err, "in UpdateJobState").Err()
	}
	shards, err := job.fetchShards(c)
	if err != nil {
		return errors.Annotate(err, "failed to fetch shards").Err()
	}

	// Switch the job into a final state only when all shards are done running.
	perState := make(map[State]int, len(State_name))
	finished := 0
	for _, sh := range shards {
		logging.Infof(c, "Shard #%d (%d) is in state %s", sh.Index, sh.ID, sh.State)
		perState[sh.State]++
		if isFinalState(sh.State) {
			finished++
		}
	}
	if finished != len(shards) {
		return nil
	}

	jobState := State_SUCCESS
	switch {
	case perState[State_ABORTED] != 0:
		jobState = State_ABORTED
	case perState[State_FAIL] != 0:
		jobState = State_FAIL
	}

	return runTxn(c, func(c context.Context) error {
		job, err := getJobInState(c, JobID(msg.JobId), State_RUNNING, State_ABORTING)
		if err != nil || job == nil {
			return errors.Annotate(err, "in UpdateJobState txn").Err()
		}

		// Make sure an aborting job ends up in aborted state, even if all its
		// shards manged to finish. It looks weird when an ABORTING job moves
		// into e.g. SUCCESS state.
		if job.State == State_ABORTING {
			job.State = State_ABORTED
		} else {
			job.State = jobState
		}
		job.Updated = clock.Now(c).UTC()

		runtime := job.Updated.Sub(job.Created)
		switch job.State {
		case State_SUCCESS:
			logging.Infof(c, "The job finished successfully in %s", runtime)
		case State_FAIL:
			logging.Errorf(c, "The job finished with %d shards failing in %s", perState[State_FAIL], runtime)
			for _, sh := range shards {
				if sh.State == State_FAIL {
					logging.Errorf(c, "Shard #%d (%d) error - %s", sh.Index, sh.ID, sh.Error)
				}
			}
		case State_ABORTED:
			logging.Warningf(c, "The job has been aborted after %s: %d shards succeeded, %d shards failed, %d shards aborted",
				runtime, perState[State_SUCCESS], perState[State_FAIL], perState[State_ABORTED])
		}

		return transient.Tag.Apply(datastore.Put(c, job))
	})
}
