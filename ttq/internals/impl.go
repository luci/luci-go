// Copyright 2020 The LUCI Authors.
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

package internals

import (
	"context"
	"encoding/hex"
	"io"
	"sync/atomic"
	"time"

	gax "github.com/googleapis/gax-go/v2"
	taskspb "google.golang.org/genproto/googleapis/cloud/tasks/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/cryptorand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/ttq"
	"go.chromium.org/luci/ttq/internals/databases"
	"go.chromium.org/luci/ttq/internals/partition"
	"go.chromium.org/luci/ttq/internals/reminder"
)

// Impl implements abstract transaction enqueue semantics on top of Database
// interface.
type Impl struct {
	Options     ttq.Options
	DB          databases.Database
	TasksClient TasksClient
}

// TasksClient decouples Impl from the Cloud Tasks client.
type TasksClient interface {
	CreateTask(context.Context, *taskspb.CreateTaskRequest, ...gax.CallOption) (*taskspb.Task, error)
}

// AddTask persists a task during a transaction and returns PostProcess to be
// called after the transaction completes (aka happy path).
func (impl *Impl) AddTask(ctx context.Context, req *taskspb.CreateTaskRequest) (ttq.PostProcess, error) {
	// TODO(tandrii): this can't be instrumented in a naive way because the
	// transaction can be retried, leading to inflated counts. Ideally, we'd want
	// a callback as soon as transaction successfully finishes or annotating
	// metrics with try number s.t. all retries can be ignored.
	r, reqClone, err := makeReminder(ctx, req)
	if err != nil {
		return nil, err
	}
	if err = impl.DB.SaveReminder(ctx, r); err != nil {
		return nil, err
	}
	once := int32(0)
	return func(outCtx context.Context) {
		if count := atomic.AddInt32(&once, 1); count > 1 {
			logging.Warningf(outCtx, "calling PostProcess %d times is not necessary", count)
			return
		}
		if clock.Now(outCtx).After(r.FreshUntil) {
			logging.Warningf(outCtx, "ttq happy path PostProcess %s has no time to run", r.Id)
			return
		}
		if err := impl.postProcess(outCtx, r, reqClone); err != nil {
			logging.Warningf(outCtx, "ttq happy path PostProcess %s failed: %s", r.Id, err)
		}
	}, nil
}

// ScanItem defines keyspace to scan for possible work.
type ScanItem struct {
	// Shard number in range of [0 .. ttq.Options.Shards).
	// Although Shard number can be calculated given the Partition and total
	// number of shards (see ttq.Options.Shards), the latter may change during
	// asynchroneous scan executions.
	Shard int
	// Partition specifies the range of keys to scan.
	Partition *partition.Partition
	// Level counts recursion level for monitoring/debugging purposes.
	// Cron triggers tasks at level=0. If there is a big backlog,
	// level=0 task will offload some work to level=1 tasks.
	// level > 1 should not normally happen and indicates either a bug or a very
	// overloaded system.
	// level > 2 won't be executed at all.
	Level int
}

// ScanResult is the result of a scan.
type ScanResult struct {
	// Shard number. See ScanItem.Shard.
	Shard int
	// Reminders are sorted by Id stale reminders without Payload.
	// Reminders belong to the partition assigned to a shard.
	Reminders []*reminder.Reminder
	// Level is the recursion level. See ScanItem.Level.
	Level int
}

// SweepAll intiates the sweeping process across the whole Reminders' keyspace.
//
// Caller is expected to call Scan() on each of the returned ScanItems.
func (impl *Impl) SweepAll() []ScanItem {
	ret := make([]ScanItem, 0, impl.Options.Shards)
	u := partition.Universe(keySpaceBytes)
	for shard, part := range u.Split(int(impl.Options.Shards)) {
		ret = append(ret, ScanItem{
			Partition: part,
			Shard:     shard,
			Level:     0,
		})
	}
	return ret
}

// Scan scans the given part of Reminders' keyspace.
//
// If Scan is unable to complete the scan of the given part of keyspace,
// it intelligently partitions the not yet scanned keyspace into several
// ScanItem for the follow up.
//
// For the successfully scanned keyspace, Scan returns a ScanResult.
// Caller is expected to call PostProcessBatch on the ScanResult.Reminders.
// NOTE: ScanResult.Reminders may be set even if the function returns an error.
// This happens if scan fetched stale Reminders before encountering the error.
func (impl *Impl) Scan(ctx context.Context, w ScanItem) ([]ScanItem, ScanResult, error) {
	switch {
	case w.Level > 2:
		return nil, ScanResult{}, errors.Reason("refusing to do level-%d scan", w.Level).Err()
	case w.Level == 2:
		logging.Warningf(ctx, "level-2 scan %s", w)
	}

	l, h := w.Partition.QueryBounds(keySpaceBytes)
	startedAt := clock.Now(ctx)
	rs, err := impl.DB.FetchRemindersMeta(ctx, l, h, int(impl.Options.TasksPerScan))
	durMS := float64(clock.Now(ctx).Sub(startedAt).Milliseconds())

	status := ""
	needMoreScans := false
	switch {
	case len(rs) > int(impl.Options.TasksPerScan):
		logging.Errorf(ctx, "bug: %s.FetchRemindersMeta returned %d > limit %d",
			impl.DB.Kind(), len(rs), impl.Options.TasksPerScan)
		fallthrough
	case len(rs) == int(impl.Options.TasksPerScan):
		status = "limit"
		// There may be more items in the partition.
		needMoreScans = true
	case err == nil:
		// Scan covered everything.
		status = "OK"
	case ctx.Err() == context.DeadlineExceeded && errors.Unwrap(err) == context.DeadlineExceeded:
		status = "timeout"
		// Nothing fetched before timeout should not happen frequently.
		// To avoid waiting until next SweepAll(), follow up with scans on
		// sub-partitions.
		needMoreScans = true
	default:
		status = "fail"
	}

	metricSweepFetchMetaDurationsMS.Add(ctx, durMS, status, w.Level, impl.DB.Kind())
	metricSweepFetchMetaReminders.Add(ctx, int64(len(rs)), status, w.Level, impl.DB.Kind())

	if !needMoreScans {
		return nil, impl.makeScanResult(ctx, w, rs), err
	}

	var scanParts partition.SortedPartitions
	if len(rs) == 0 {
		// Divide initial range.
		scanParts = w.Partition.Split(int(impl.Options.SecondaryScanShards))
	} else {
		// Divide the range after the last fetched Reminder.
		scanParts = w.Partition.EducatedSplitAfter(
			rs[len(rs)-1].Id,
			len(rs),
			// Aim to hit these many Reminders per follow up ScanItem,
			int(impl.Options.TasksPerScan),
			// but create at most these many ScanItems.
			int(impl.Options.SecondaryScanShards),
		)
	}
	moreScans := make([]ScanItem, len(scanParts))
	for i, part := range scanParts {
		moreScans[i] = ScanItem{
			Shard:     w.Shard,
			Level:     w.Level + 1,
			Partition: part,
		}
	}
	return moreScans, impl.makeScanResult(ctx, w, rs), err
}

// PostProcessBatch efficiently processes a batch of stale Reminders.
//
// Reminders batch will be modified to fetch Reminders' Payload.
//
// RAM usage is equivalent to O(total Payload size of each Reminder in batch).
func (impl *Impl) PostProcessBatch(ctx context.Context, batch []*reminder.Reminder) error {
	payloaded, err := impl.DB.FetchReminderPayloads(ctx, batch)
	switch missing := len(batch) - len(payloaded); {
	case missing < 0:
		panic(errors.Reason("%s.FetchReminderPayloads returned %d but asked for %d Reminders",
			impl.DB.Kind(), len(payloaded), len(batch)).Err())
	case err != nil:
		logging.Warningf(ctx, "failed to fetch %d/%d Reminders: %s", missing, len(batch), err)
		// Continue PostProcess-ing whatever was fetched anyway.
	case missing > 0:
		logging.Warningf(ctx, "%d stale Reminders were unexpectedly deleted by something else. "+
			"If this persists, check for a misconfiguration of the sweeping or the happy path timeout",
			missing)
	}

	// This can be optimized further by batching deletion of Reminders,
	// but the current version was good enough in load tests already.
	merr := parallel.WorkPool(16, func(workChan chan<- func() error) {
		for _, r := range payloaded {
			r := r
			workChan <- func() error { return impl.postProcess(ctx, r, nil) }
		}
	})
	switch {
	case err == nil:
		return merr
	case merr == nil:
		return err
	default:
		e := merr.(errors.MultiError)
		return append(e, err)
	}
}

// Helpers.

// postProcess enqueues a task and deletes a reminder.
// During happy path, the req is not nil.
func (impl *Impl) postProcess(ctx context.Context, r *reminder.Reminder, req *taskspb.CreateTaskRequest) (err error) {
	startedAt := clock.Now(ctx)
	when := "happy"
	status := "OK"
	defer func() {
		durMS := float64(clock.Now(ctx).Sub(startedAt).Milliseconds())
		metricPostProcessedAttempts.Add(ctx, 1, status, when, impl.DB.Kind())
		metricPostProcessedDurationsMS.Add(ctx, durMS, status, when, impl.DB.Kind())
	}()

	if req == nil {
		when = "sweep"
		req = &taskspb.CreateTaskRequest{}
		if err = proto.Unmarshal(r.Payload, req); err != nil {
			status = "fail-deserialize"
			err = errors.Annotate(err, "failed to unmarshal the task").Err()
			return
		}
	}

	switch err = impl.createCloudTask(ctx, req, when); {
	case err == nil:
	case transient.Tag.In(err):
		status = "fail-task"
		return err
	default:
		status = "ignored-bad-task"
		// Proceed deleting reminder, there is no point retrying later.
	}
	if err = impl.DB.DeleteReminder(ctx, r); err != nil {
		// This may override "ignored-bad-task" status and this is fine as:
		//   * createCloudTask keeps metrics, too
		//   * most likely the reminder wasn't deleted,
		//     so there will be a retry later anyway.
		status = "fail-db"
		return err
	}
	return nil
}

// createCloudTask tries to create a Cloud Task.
// On AlreadyExists error, returns nil.
// The returned error has transient.Tag applied if the error isn't permanent.
func (impl *Impl) createCloudTask(ctx context.Context, req *taskspb.CreateTaskRequest, when string) error {
	// WORKAROUND(https://github.com/googleapis/google-cloud-go/issues/1577): if
	// the passed context deadline is larger than 30s, the CreateTask call fails
	// with InvalidArgument "The request deadline is ... The deadline cannot be
	// more than 30s in the future." So, give it 20s.
	ctx, cancel := context.WithTimeout(ctx, time.Second*20)
	defer cancel()
	_, err := impl.TasksClient.CreateTask(ctx, req)
	code := status.Code(err)
	switch code {
	case codes.OK:
	case codes.AlreadyExists:
		err = nil

	case codes.InvalidArgument:
		err = errors.Annotate(err, "failed to create Cloud Task").Err()

	case codes.Unavailable:
		fallthrough
	default:
		// TODO(tandrii): refine with use which errors really should be fatal and
		// which should not.
		err = errors.Annotate(err, "failed to create Cloud Task").Tag(transient.Tag).Err()
	}
	if when != "ttq" {
		// Monitor only tasks created for the user.
		metricTasksCreated.Add(ctx, 1, code.String(), when, impl.DB.Kind())
	}
	return err
}

// makeScanResult updates metricReminderAge based on all fetched reminders.
// Returns ScanResult with just stale Reminders.
// Mutates & re-uses the given Reminders slice.
func (impl *Impl) makeScanResult(ctx context.Context, s ScanItem, reminders []*reminder.Reminder) ScanResult {
	dbKind := impl.DB.Kind()
	now := clock.Now(ctx)
	i := 0
	for _, r := range reminders {
		staleness := now.Sub(r.FreshUntil)
		metricReminderStalenessMS.Add(ctx, float64(staleness.Milliseconds()), s.Level, dbKind)
		if staleness >= 0 {
			reminders[i] = r
			i++
		}
	}
	return ScanResult{
		Shard:     s.Shard,
		Level:     s.Level,
		Reminders: reminders[:i],
	}
}

const (
	// keySpaceBytes defines the space of the Reminder Ids.
	//
	// Because Reminder.Id is hex-encoded, actual len(Ids) == 2*keySpaceBytes.
	//
	// 16 is chosen is big enough to avoid collisions in practice yet small enough
	// for easier human-debugging of key ranges in queries.
	keySpaceBytes = 16

	// happyPathMaxDuration caps how long the happy path will be waited for.
	happyPathMaxDuration = time.Minute
)

// makeReminder creates a Reminder and a cloned CreateTaskRequest with a named
// task.
//
// The request is cloned to avoid bugs when user's transaction code is retried.
// The resulting request's task is then named to avoid avoid duplicate task
// creation later on.
// The resulting cloned request is returned to avoid needlessly deserializing it
// from the Reminder.Payload in the PostProcess callback.
func makeReminder(ctx context.Context, req *taskspb.CreateTaskRequest) (*reminder.Reminder, *taskspb.CreateTaskRequest, error) {
	if req.Task == nil {
		return nil, nil, errors.New("CreateTaskRequest.Task must be set")
	}
	if req.Task.Name != "" {
		return nil, nil, errors.New("CreateTaskRequest.Task.Name must not be set. Named tasks are not supported")
	}
	buf := make([]byte, keySpaceBytes)
	if _, err := io.ReadFull(cryptorand.Get(ctx), buf); err != nil {
		return nil, nil, errors.Annotate(err, "failed to get random bytes").Tag(transient.Tag).Err()
	}

	r := &reminder.Reminder{Id: hex.EncodeToString(buf)}
	// Bound FreshUntil to at most current context deadline.
	r.FreshUntil = clock.Now(ctx).Add(happyPathMaxDuration)
	if deadline, ok := ctx.Deadline(); ok && r.FreshUntil.After(deadline) {
		// TODO(tandrii): allow propagating custom deadline for the async happy
		// path which won't bind the context's deadline.
		r.FreshUntil = deadline
	}
	r.FreshUntil = r.FreshUntil.UTC().Truncate(reminder.FreshUntilPrecision)

	clone := proto.Clone(req).(*taskspb.CreateTaskRequest)
	clone.Task.Name = clone.Parent + "/tasks/" + r.Id
	var err error
	if r.Payload, err = proto.Marshal(clone); err != nil {
		return nil, nil, errors.Annotate(err, "failed to marshal the task").Err()
	}
	return r, clone, nil
}

// OnlyLeased shrinks the given slice of Reminders sorted by their ID to contain
// only ones that belong to any of the leased partitions.
func OnlyLeased(sorted []*reminder.Reminder, leased partition.SortedPartitions, keySpaceBytes int) []*reminder.Reminder {
	reuse := sorted[:]
	l := 0
	keyOf := func(i int) string {
		return sorted[i].Id
	}
	use := func(i, j int) {
		l += copy(reuse[l:], sorted[i:j])
	}
	leased.OnlyIn(len(sorted), keyOf, use, keySpaceBytes)
	return reuse[:l]
}
