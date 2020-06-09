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

// Package ttq implements transaction task queues on Google Cloud.
package ttq

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	taskspb "google.golang.org/genproto/googleapis/cloud/tasks/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"

	"go.chromium.org/luci/ttq/internal"
)

type PostProcess func(context.Context)

const MPrefix = "experimental/crbug-1080423/1/"

var metricPostprocessed = metric.NewCounter(
	MPrefix+"ttq/postprocessed",
	"Post Processing attempts",
	nil,
	field.String("status"),
	field.String("when"),
	field.String("db"),
)
var metricTaskCreate = metric.NewCounter(
	MPrefix+"ttq/created",
	"Number of user tasks created",
	nil,
	field.String("status"),
	field.String("when"),
	field.String("db"),
)

var metricShardFetchDuration = metric.NewCumulativeDistribution(
	MPrefix+"ttq/shard/fetch/duration",
	"How long it took to fetch just the keys (ms)",
	&types.MetricMetadata{Units: types.Milliseconds},
	distribution.DefaultBucketer,
	field.Int("level"),
	field.String("db"),
)

var metricShardFetchCount = metric.NewCumulativeDistribution(
	MPrefix+"ttq/shard/fetch/count",
	"How many keys the shard fetched",
	nil,
	distribution.DefaultBucketer,
	field.Int("level"),
	field.String("db"),
)

var metricLeaseHoldDuration = metric.NewCumulativeDistribution(
	MPrefix+"ttq/lease/hold/duration",
	"How long the lease was held for (ms)",
	&types.MetricMetadata{Units: types.Milliseconds},
	distribution.DefaultBucketer,
	field.Int("level"),
	field.String("db"),
)

var metricLeaseFetched = metric.NewCumulativeDistribution(
	MPrefix+"ttq/lease/fetched",
	"Number of leases fetched",
	nil,
	distribution.DefaultBucketer,
	field.String("status"),
	field.String("db"),
)

var metricLeaseObtainDuration = metric.NewCumulativeDistribution(
	MPrefix+"ttq/lease/obtain/duration",
	"How long it took to obtain the lease (ms)",
	&types.MetricMetadata{Units: types.Milliseconds},
	distribution.DefaultBucketer,
	field.String("status"),
	field.String("db"),
)

var metricReminderAge = metric.NewCumulativeDistribution(
	MPrefix+"ttq/reminder/age",
	"Age of discovered reminders",
	&types.MetricMetadata{Units: types.Milliseconds},
	distribution.DefaultBucketer,
	field.Int("level"),
	field.String("db"),
)

type TTQ struct {
	// reminderQueue is the queue name used by ttq library implementation.
	//
	// `projects/PROJECT_ID/locations/LOCATION_ID/queues/QUEUE_ID`
	reminderQueue string

	// baseURL is the URL which application will delegate handling to
	// the ttq library.
	baseURL string

	ctc *cloudtasks.Client
	db  Database
}

func New(queue string, baseURL string, ctc *cloudtasks.Client, d Database) *TTQ {
	return &TTQ{queue, baseURL, ctc, d}
}

type Database interface {
	SaveReminder(context.Context, *internal.Reminder) error
	DeleteReminder(context.Context, *internal.Reminder) error
	FetchReminderMeta(ctx context.Context, low, high string, limit int) ([]*internal.Reminder, error)
	FetchReminderPayloads(context.Context, []*internal.Reminder) ([]*internal.Reminder, error)

	RunInTransaction(context.Context, func(context.Context) error) error
	LoadLeases(context.Context, int) ([]*internal.Lease, error)
	SaveLease(context.Context, *internal.Lease, int) error
	ReturnLease(context.Context, *internal.Lease) error
	DeleteExpiredLeases(context.Context, []*internal.Lease) error

	Kind() string
}

const happyPathDuration = time.Minute

const keySpaceBytes = 16 // Half of SHA2.

func (s *TTQ) makeReminder(ctx context.Context, task *taskspb.Task) (*internal.Reminder, error) {
	if task.Name == "" {
		panic("TODO: implement tasks without de-duplication")
	}
	h := sha256.New()
	h.Write([]byte(task.Name))
	r := &internal.Reminder{
		FreshUntil: clock.Now(ctx).Add(happyPathDuration).UTC(),
	}
	r.ID = hex.EncodeToString(h.Sum(nil))[:keySpaceBytes*2]
	var err error
	if r.Payload, err = proto.Marshal(task); err != nil {
		return nil, errors.Annotate(err, "failed to marshal the task").Err()
	}
	return r, nil
}

func (s *TTQ) Enqueue(ctx context.Context, task *taskspb.Task) (PostProcess, error) {
	r, err := s.makeReminder(ctx, task)
	if err != nil {
		return nil, err
	}
	if err = s.db.SaveReminder(ctx, r); err != nil {
		return nil, err
	}
	return func(outsideTransaction context.Context) {
		if err := s.postProcess(outsideTransaction, r, task); err != nil {
			logging.Warningf(ctx, "happy path failed %s: %s", r.ID, err)
		} else {
			// logging.Debugf(ctx, "happy path succeeded %s", d.ID)
		}
	}, nil
}

func (s *TTQ) postProcess(ctx context.Context, r *internal.Reminder, task *taskspb.Task) error {
	when := "happy"
	if task == nil {
		// The sweeper must obtain actual task from DSReminder.
		task = &taskspb.Task{}
		if err := proto.Unmarshal(r.Payload, task); err != nil {
			return errors.Annotate(err, "failed to unmarshal the task").Err()
		}
		when = "unhappy"
	}
	// logging.Debugf(ctx, "task is %v", task)
	req := taskspb.CreateTaskRequest{
		Parent: task.Name[0:strings.LastIndex(task.Name, "/tasks/")],
		Task:   task,
	}
	if err := s.createCloudTask(ctx, &req, when); err != nil {
		metricPostprocessed.Add(ctx, 1, "fail-task", when, s.db.Kind())
		return errors.Annotate(err, "failed to create the cloud task").Err()
	}
	if err := s.db.DeleteReminder(ctx, r); err != nil {
		metricPostprocessed.Add(ctx, 1, "fail-db", when, s.db.Kind())
		return err
	}
	metricPostprocessed.Add(ctx, 1, "ok", when, s.db.Kind())
	return nil
}

const defaultShardsNumber = 16

// shardSweepItemsLimit is the number of keys that can be fetched & processed
// within the 1 minute deadline by a single shard worker.
//
// This is determined practically via a load test.
const shardSweepItemsLimit = 2048
const tooLargePartitionSplitInto = 4
const maxShardFanout = 128
const batchSize = 50
const sweepFrequency = 1 * time.Minute

func (s *TTQ) scheduleSweepShard(ctx context.Context, shardId, level int, partition *internal.Partition) error {
	var scheduleTime *timestamppb.Timestamp
	name := ""

	if level == 0 {
		// Dedup level-0 requests.
		now := clock.Now(ctx).UTC()
		eta := now.Round(sweepFrequency)
		if eta.Before(now) {
			eta = eta.Add(sweepFrequency)
		}
		name = fmt.Sprintf("%s/tasks/%d-%s", s.reminderQueue, eta.Unix(), partition)
		scheduleTime = google.NewTimestamp(eta)
	}

	req := taskspb.CreateTaskRequest{
		Parent: s.reminderQueue,
		Task: &taskspb.Task{
			Name: name,
			MessageType: &taskspb.Task_HttpRequest{HttpRequest: &taskspb.HttpRequest{
				HttpMethod: taskspb.HttpMethod_GET,
				Url:        fmt.Sprintf("%s/shard/%d_%d_%s", s.baseURL, shardId, level, partition.String()),
			}},
			ScheduleTime: scheduleTime,
			// TODO
			// AuthorizationHeader: &HttpRequest_OidcToken{OidcToken: &taskspb.OidcToken{
			// 	ServiceAccountEmail: "TODO",
			// 	Audience:            "",
			// }},
		},
	}
	return s.createCloudTask(ctx, &req, "sys")
}

func (s *TTQ) Sweep(ctx context.Context) error {
	u := internal.Universe(keySpaceBytes)
	level := 0
	for shardId, part := range u.Split(defaultShardsNumber) {
		if err := s.scheduleSweepShard(ctx, shardId, level, part); err != nil {
			return err
		}
	}
	return nil
}

func (s *TTQ) SweepShard(ctx context.Context, shard string) error {
	startedAt := clock.Now(ctx)
	availableTime := time.Minute
	if deadline, ok := ctx.Deadline(); ok {
		availableTime = deadline.Sub(startedAt)
	}
	shardParts := strings.SplitN(shard, "_", 3)
	shardId, err := strconv.Atoi(shardParts[0])
	if err != nil {
		return err
	}
	level, err := strconv.Atoi(shardParts[1])
	if err != nil {
		return err
	}
	partition, err := internal.PartitionFromString(shardParts[2])
	if err != nil {
		return err
	}
	// logging.Debugf(ctx, "working on %v", partition.String())

	items := []*internal.Reminder{}
	err = func() (err error) {
		// Limit max time spent scanning.
		innerCtx, cancel := context.WithTimeout(ctx, availableTime/5)
		defer cancel()
		l, h := partition.QueryBounds(keySpaceBytes)
		items, err = s.db.FetchReminderMeta(innerCtx, l, h, shardSweepItemsLimit)
		return
	}()
	fetchCompletedAt := clock.Now(ctx)
	took := fetchCompletedAt.Sub(startedAt)
	metricShardFetchDuration.Add(ctx, float64(took.Milliseconds()), level, s.db.Kind())
	metricShardFetchCount.Add(ctx, float64(len(items)), level, s.db.Kind())
	logging.Debugf(ctx, "fetched %d items, err %s [took: %s]", len(items), err, took)

	var followUp internal.SortedPartitions
	switch {
	case len(items) > 0:
		if err != nil || len(items) == shardSweepItemsLimit {
			// There may be more items in the partition.
			lastFound := items[len(items)-1].ID
			followUp = partition.EducatedSplitAfter(lastFound, len(items), shardSweepItemsLimit, maxShardFanout)
		}
	case err == nil:
		// Nothing to do.
		return nil
	case err == context.DeadlineExceeded:
		// This can be a fluke or it can be that the search range is too large to
		// scan before the deadline, which can happen if compaction didn't have time
		// to catch with the deletion, resulting in query scanning over lots of
		// deleted entries.
		followUp = partition.Split(tooLargePartitionSplitInto)
	default:
		// Assume transient error and retry this task.
		return err
	}
	return transient.Tag.Apply(parallel.FanOutIn(func(workChan chan<- func() error) {
		for _, part := range followUp {
			part := part
			workChan <- func() error { return s.scheduleSweepShard(ctx, shardId, level+1, part) }
		}
		if len(items) > 0 {
			workChan <- func() error {
				return s.sweepReminders(ctx, shardId, level, items)
			}
		}
	}))
}

func onlyStale(ctx context.Context, level int, db string, reminders []*internal.Reminder) []*internal.Reminder {
	now := clock.Now(ctx)
	i := 0
	for _, r := range reminders {
		age := now.Sub(r.FreshUntil)
		if age >= 0 {
			reminders[i] = r
			i++
			metricReminderAge.Add(ctx, float64(age.Milliseconds()), level, db)
		}
	}
	return reminders[:i]
}

func (s *TTQ) sweepReminders(ctx context.Context, shardId, level int, reminders []*internal.Reminder) error {
	reminders = onlyStale(ctx, level, s.db.Kind(), reminders)
	if len(reminders) == 0 {
		logging.Debugf(ctx, "no stale reminders")
		return nil
	}

	desired := internal.PartitionSpanning(reminders)
	logging.Debugf(ctx, "about to lease for %s with %d items", desired.String(), len(reminders))
	err := s.withLease(ctx, desired, shardId, func(ctx context.Context, leased internal.SortedPartitions) error {
		tStart := clock.Now(ctx)
		defer func() {
			metricLeaseHoldDuration.Add(ctx, float64(clock.Now(ctx).Sub(tStart).Milliseconds()), level, s.db.Kind())
		}()
		// partition obtained may be smaller than requested.
		reminders = leased.Filter(reminders, keySpaceBytes)
		logging.Debugf(ctx, "leased %s covering %d items", leased, len(reminders))
		return parallel.FanOutIn(func(workChan chan<- func() error) {
			for len(reminders) > 0 {
				count := batchSize
				if count > len(reminders) {
					count = len(reminders)
				}
				var batch []*internal.Reminder
				batch, reminders = reminders[:count], reminders[count:]
				workChan <- func() error { return s.sweepRemindersBatch(ctx, batch) }
			}
		})
	})
	if err == ErrInLease {
		logging.Warningf(ctx, "key range %s is already leased by other request(s)", desired.String())
		return nil
	}
	return err
}

func (s *TTQ) sweepRemindersBatch(ctx context.Context, batch []*internal.Reminder) error {
	now := clock.Now(ctx)
	batch, err := s.db.FetchReminderPayloads(ctx, batch)
	// If anything was read, execute it regardless of other errors.
	merr := parallel.FanOutIn(func(workChan chan<- func() error) {
		for _, item := range batch {
			switch {
			case now.Before(item.FreshUntil):
				// Give happy path a chance and decrease probability of calling
				// CloudTasks API twice for the same task.
			default:
				item := item
				workChan <- func() error { return s.postProcess(ctx, item, nil) }
			}
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

var ErrInLease = errors.New("range is already being leased by someone")

func (s *TTQ) withLease(ctx context.Context, desired *internal.Partition, shardId int,
	clbk func(context.Context, internal.SortedPartitions) error) error {
	// Read outside transaction to fail quickly and cheaply if lease isn't
	// feasible.
	switch can, err := s.canLease(ctx, desired, shardId); {
	case err != nil:
		return err
	case !can:
		return ErrInLease
	}

	tStart := clock.Now(ctx)
	deadline, ok := ctx.Deadline()
	innerCtx := ctx
	if !ok || deadline.Sub(tStart) > time.Minute {
		// Limit the lease duration.
		var cancel func()
		innerCtx, cancel = context.WithTimeout(ctx, time.Minute)
		defer cancel()
		deadline, _ = innerCtx.Deadline()
	}

	l, sp, err := s.doLease(ctx, desired, shardId, deadline)
	took := float64(clock.Now(ctx).Sub(tStart).Milliseconds())
	switch {
	case err != nil:
		metricLeaseObtainDuration.Add(ctx, took, "err", s.db.Kind())
		return err
	case l == nil:
		metricLeaseObtainDuration.Add(ctx, took, "inlease", s.db.Kind())
		return ErrInLease
	default:
		metricLeaseObtainDuration.Add(ctx, took, "ok", s.db.Kind())
		defer func() {
			if err := s.db.ReturnLease(ctx, l); err != nil {
				logging.Warningf(ctx, "ignoring failure to return the lease: %s", err)
			}
		}()
		return clbk(innerCtx, sp)
	}
}

func (s *TTQ) createCloudTask(ctx context.Context, req *taskspb.CreateTaskRequest, when string) error {
	// WORKAROUND(https://github.com/googleapis/google-cloud-go/issues/1577): if
	// the passed context deadline is larger than 30s, the CreateTask call fails
	// with InvalidArgument "The request deadline is ... The deadline cannot be
	// more than 30s in the future." So, give it 20s.
	ctx, cancel := context.WithTimeout(ctx, time.Second*20)
	defer cancel()
	// logging.Debugf(ctx, "creating named task %s", req.Task.Name)
	_, err := s.ctc.CreateTask(ctx, req)
	code := status.Code(err)
	if code == codes.AlreadyExists {
		if when != "sys" {
			logging.Warningf(ctx, "task %s already exists", req.Task.Name)
		}
		err = nil
	}
	metricTaskCreate.Add(ctx, 1, code.String(), when, s.db.Kind())
	return err
}

func (s *TTQ) loadLeasesInstrumented(ctx context.Context, shardId int) (leases []*internal.Lease, err error) {
	leases, err = s.db.LoadLeases(ctx, shardId)
	status := "ok"
	if err != nil {
		status = "err"
	}
	metricLeaseFetched.Add(ctx, float64(len(leases)), status, s.db.Kind())
	return
}

func (s *TTQ) canLease(ctx context.Context, desired *internal.Partition, shardId int) (bool, error) {
	leases, err := s.loadLeasesInstrumented(ctx, shardId)
	if err != nil {
		return false, err
	}
	l, _, _ := internal.LeaseMaxPossible(ctx, leases, desired, time.Time{})
	return l != nil, nil
}

func (t *TTQ) doLease(ctx context.Context, desired *internal.Partition, shardId int, expiresAt time.Time) (
	l *internal.Lease, sp internal.SortedPartitions, err error) {
	var expiredLeases []*internal.Lease
	err = t.db.RunInTransaction(ctx, func(ctx context.Context) (err error) {
		leases, err := t.loadLeasesInstrumented(ctx, shardId)
		if err != nil {
			return err
		}
		l, sp, expiredLeases = internal.LeaseMaxPossible(ctx, leases, desired, expiresAt)
		if l == nil {
			return ErrInLease
		}
		if err := t.db.SaveLease(ctx, l, shardId); err != nil {
			return err
		}
		if expiredLeases != nil {
			// Deleting >= 1 lease every time a new one is created suffices to avoid
			// accumulating garbage above O(active leases).
			if len(expiredLeases) > 50 {
				expiredLeases = expiredLeases[:50]
			}
			logging.Warningf(ctx, "deleting %d expired leases", len(expiredLeases))
			if err = t.db.DeleteExpiredLeases(ctx, expiredLeases); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		return
	}
	l = nil
	sp = nil
	if uerr := errors.Unwrap(err); uerr == ErrInLease {
		err = ErrInLease
	} else {
		err = errors.Annotate(err, "failed to transact a lease").Tag(transient.Tag).Err()
	}
	return
}
