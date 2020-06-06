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
	"github.com/google/uuid"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	taskspb "google.golang.org/genproto/googleapis/cloud/tasks/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/gae/service/datastore"
	ds "go.chromium.org/gae/service/datastore"
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
)

type PostProcess func(context.Context)

/*
type Spanner struct {
	table string
}

func (s *Spanner) Enqueue(ctx context.Context, tctx *spanner.ReadWriteTransaction, ctc *cloudtasks.Client, queue string, task *taskspb.Task) (PostProcess, error) {
	return nil, nil
}
*/

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

const DbDatastore = "datastore"
const DbSpanner = "spanner"

// Reminder is a record of transactionally enqueued task.
type Reminder struct {
	_kind string `gae:"$kind,ttq.Task"`
	// ID is a hash of the task name and it is designed to be well
	// distributed in keyspace to avoid hotspots.
	ID string `gae:"$id"`
	// FreshUntil is the expected time by which happy path should complete.
	//
	// The sweeper will ignore a DSReminder while it is fresh.
	FreshUntil time.Time `gae:",noindex"`

	// Payload is a serialized taskspb.Task.
	Payload []byte `gae:",noindex"`
}

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

type Database interface {
	SaveReminder(context.Context, *Reminder) error
	DeleteReminder(context.Context, *Reminder) error
	FetchReminderKeys(ctx context.Context, low, high string) ([]*Reminder, error)
	FetchReminders(context.Context, []*Reminder) error

	IsErrNoSuchReminder(error) bool

	Kind() string
}

type Datastore struct{}

func (d *Datastore) Blah() {}

func NewDatastore(queue string, baseURL string, ctc *cloudtasks.Client) *TTQ {
	return &TTQ{queue, baseURL, ctc, &Datastore{}}
}

const happyPathDuration = time.Minute

const keySpaceBytes = 16 // Half of SHA2.

func (d *Datastore) Kind() string {
	return DbDatastore
}

func (d *Datastore) SaveReminder(ctx context.Context, r *Reminder) error {
	if err := ds.Put(ctx, r); err != nil {
		return errors.Annotate(err, "failed to persist to datastore").Err()
	}
	return nil
}
func (d *Datastore) DeleteReminder(ctx context.Context, r *Reminder) error {
	if err := ds.Delete(ctx, r); err != nil {
		return errors.Annotate(err, "failed to delete the Reminder %s", r.ID).Err()
	}
	return nil
}

func (d *Datastore) FetchReminders(ctx context.Context, batch []*Reminder) error {
	return ds.Get(ctx, batch)
}
func (d *Datastore) FetchReminderKeys(ctx context.Context, low, high string) ([]*Reminder, error) {
	q := ds.NewQuery("ttq.Task").Order("__key__")
	q = q.Gte("__key__", ds.KeyForObj(ctx, &Reminder{ID: low}))
	q = q.Lt("__key__", ds.KeyForObj(ctx, &Reminder{ID: high}))
	q = q.Limit(shardSweepItemsLimit).KeysOnly(true)
	logging.Debugf(ctx, "query: %s..%s limit %s", low, high, shardSweepItemsLimit)
	var items []*Reminder
	err := ds.Run(ctx, q, func(k *ds.Key) {
		items = append(items, &Reminder{ID: k.StringID()})
	})
	switch {
	case err == nil:
		return items, nil
	case err == context.DeadlineExceeded:
		return items, err
	default:
		return items, errors.Annotate(err, "failed to fetch reminder keys").Tag(transient.Tag).Err()
	}
}

func (d *Datastore) IsErrNoSuchReminder(err error) bool {
	return ds.IsErrNoSuchEntity(err)
}

func (s *TTQ) makeReminder(ctx context.Context, task *taskspb.Task) (*Reminder, error) {
	if task.Name == "" {
		panic("TODO: implement tasks without de-duplication")
	}
	h := sha256.New()
	h.Write([]byte(task.Name))
	r := &Reminder{
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

func (s *TTQ) postProcess(ctx context.Context, r *Reminder, task *taskspb.Task) error {
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

func (s *TTQ) scheduleSweepShard(ctx context.Context, shardId, level int, partition *Partition) error {
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
	u := Universe(keySpaceBytes)
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
	partition, err := PartitionFromString(shardParts[2])
	if err != nil {
		return err
	}
	// logging.Debugf(ctx, "working on %v", partition.String())

	items := []*Reminder{}
	err = func() (err error) {
		// Limit max time spent scanning.
		innerCtx, cancel := context.WithTimeout(ctx, availableTime/5)
		defer cancel()
		l, h := partition.queryBounds(keySpaceBytes)
		items, err = s.db.FetchReminderKeys(innerCtx, l, h)
		return
	}()
	fetchCompletedAt := clock.Now(ctx)
	took := fetchCompletedAt.Sub(startedAt)
	metricShardFetchDuration.Add(ctx, float64(took.Milliseconds()), level, s.db.Kind())
	metricShardFetchCount.Add(ctx, float64(len(items)), level, s.db.Kind())
	logging.Debugf(ctx, "fetched %d items, err %s [took: %s]", len(items), err, took)

	var followUp SortedPartitions
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
				return s.sweepItems(ctx, shardId, level, items)
			}
		}
	}))
}

func (s *TTQ) sweepItems(ctx context.Context, shardId, level int, items []*Reminder) error {
	desired := PartitionSpanning(items)
	logging.Debugf(ctx, "about to lease for %s", desired.String())
	err := s.RunLease(ctx, desired, shardId, func(ctx context.Context, leased SortedPartitions) error {
		tLeaseStart := clock.Now(ctx)
		defer func() {
			metricLeaseHoldDuration.Add(ctx, float64(clock.Now(ctx).Sub(tLeaseStart).Milliseconds()), level, DbDatastore)
		}()
		// Partition obtained may be smaller than requested.
		items = leased.filter(items, keySpaceBytes)
		logging.Debugf(ctx, "filtered to %d due to lease %s", len(items), leased)
		return parallel.FanOutIn(func(workChan chan<- func() error) {
			for len(items) > 0 {
				count := batchSize
				if count > len(items) {
					count = len(items)
				}
				var batch []*Reminder
				batch, items = items[:count], items[count:]
				workChan <- func() error { return s.sweepOneBatch(ctx, batch) }
			}
		})
	})
	if err == ErrInLease {
		logging.Warningf(ctx, "key range %s is already leased by other request(s)", desired.String())
		return nil
	}
	return err
}

func (s *TTQ) sweepOneBatch(ctx context.Context, batch []*Reminder) error {
	now := clock.Now(ctx)
	err := s.db.FetchReminders(ctx, batch)
	merr, ok := err.(errors.MultiError)
	if !ok && err != nil {
		return errors.Annotate(err, "failed to fetch %d reminders", len(batch)).Tag(transient.Tag).Err()
	}
	return parallel.FanOutIn(func(workChan chan<- func() error) {
		for i, item := range batch {
			switch {
			case merr != nil && s.db.IsErrNoSuchReminder(merr[i]):
				// It's expected that some items will be deleted by via happy path,
				// so nothing to do here.
			case now.Before(item.FreshUntil):
				// Give happy path a chance and decrease probability of calling
				// CloudTasks API twice for the same task.
			default:
				item := item
				workChan <- func() error { return s.postProcess(ctx, item, nil) }
			}
		}
	})
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

type LeasesRoot struct {
	_kind string `gae:"$kind,ttq.LeasesRoot"`
	ID    string `gae:"$id"`
}

func leasesRootEntity(shardId int) *LeasesRoot {
	return &LeasesRoot{ID: strconv.Itoa(shardId)}
}

func leasesRootKey(ctx context.Context, shardId int) *datastore.Key {
	return datastore.KeyForObj(ctx, leasesRootEntity(shardId))
}

type Lease struct {
	_kind  string         `gae:"$kind,ttq.Lease"`
	ID     string         `gae:"$id"`
	Parent *datastore.Key `gae:"$parent"`

	Parts     []string  `gae:",noindex"` // each el is a Partition.String()
	ExpiresAt time.Time `gae:",noindex"`
}

func loadLeases(ctx context.Context, shardIndex int) (leases []*Lease, err error) {
	q := datastore.NewQuery("ttq.Lease").Ancestor(leasesRootKey(ctx, shardIndex))
	status := "ok"
	if err = ds.GetAll(ctx, q, &leases); err != nil {
		status = "err"
		err = errors.Annotate(err, "failed to fetch leases").Tag(transient.Tag).Err()
	}
	metricLeaseFetched.Add(ctx, float64(len(leases)), status, DbDatastore)
	return
}

var ErrInLease = errors.New("range is already being leased by someone")

func (s *TTQ) RunLease(ctx context.Context, desired *Partition, shardId int, clbk func(context.Context, SortedPartitions) error) error {
	startedAt := clock.Now(ctx)
	deadline, ok := ctx.Deadline()
	innerCtx := ctx
	if !ok || deadline.Sub(startedAt) > time.Minute {
		// Limit the lease duration.
		var cancel func()
		innerCtx, cancel = context.WithTimeout(ctx, time.Minute)
		defer cancel()
		deadline, _ = innerCtx.Deadline()
	}

	// Read outside transaction to fail quickly and cheaply if lease isn't
	// feasible.
	switch can, err := canLease(ctx, desired, shardId); {
	case err != nil:
		return err
	case !can:
		return ErrInLease
	}

	tStart := clock.Now(ctx)
	l, sp, err := doLease(ctx, desired, shardId, deadline)
	tLeaseStart := clock.Now(ctx)
	took := float64(tLeaseStart.Sub(tStart).Milliseconds())
	if err != nil {
		metricLeaseObtainDuration.Add(ctx, took, "err", DbDatastore)
		return err
	}
	if l == nil {
		metricLeaseObtainDuration.Add(ctx, took, "inlease", DbDatastore)
		return ErrInLease
	}
	metricLeaseObtainDuration.Add(ctx, took, "ok", DbDatastore)
	defer func() {
		if err := returnLease(ctx, l); err != nil {
			logging.Warningf(ctx, "ignoring failure to return the lease: %s", err)
		}
	}()
	return clbk(innerCtx, sp)
}

func canLease(ctx context.Context, desired *Partition, shardId int) (bool, error) {
	leases, err := loadLeases(ctx, shardId)
	if err != nil {
		return false, err
	}
	l, _, _ := availableLease(ctx, leases, desired, time.Time{})
	return l != nil, nil
}

func doLease(ctx context.Context, desired *Partition, shardId int, expiresAt time.Time) (l *Lease, sp SortedPartitions, err error) {
	var expiredLeases []*Lease
	err = ds.RunInTransaction(ctx, func(ctx context.Context) (err error) {
		leases, err := loadLeases(ctx, shardId)
		if err != nil {
			return err
		}
		l, sp, expiredLeases = availableLease(ctx, leases, desired, expiresAt)
		if l == nil {
			return ErrInLease
		}
		u, err := uuid.NewRandom()
		if err != nil {
			return errors.Annotate(err, "failed to generate lease ID").Tag(transient.Tag).Err()
		}
		l.ID = u.String()
		l.Parent = leasesRootKey(ctx, shardId)
		if err = ds.Put(ctx, leasesRootEntity(shardId), l); err != nil {
			return errors.Annotate(err, "failed to save a new lease").Tag(transient.Tag).Err()
		}
		if expiredLeases != nil {
			// Deleting >= 1 lease every time a new one is created suffices to avoid
			// accumulating garbage above O(active leases).
			if len(expiredLeases) > 50 {
				expiredLeases = expiredLeases[:50]
			}
			logging.Debugf(ctx, "deleting %d expired...", len(expiredLeases))
			if err = ds.Delete(ctx, expiredLeases); err != nil {
				return errors.Annotate(err, "failed to delete %d expired leases", len(expiredLeases)).Err()
			}
		}
		return nil
	}, &ds.TransactionOptions{Attempts: 5})
	if err == nil {
		return
	}
	l = nil
	sp = nil
	if err != ErrInLease {
		err = errors.Annotate(err, "failed to transact a lease").Tag(transient.Tag).Err()
	}
	return
}

func returnLease(ctx context.Context, l *Lease) error {
	// Transaction isn't neccessary -- it's fine if leasing decision of ongoing
	// transaction is made as if this lease is still active.
	if err := ds.Delete(ctx, l); err != nil {
		return errors.Annotate(err, "failed to delete the Lease entity").Err()
	}
	return nil
}

func availableLease(ctx context.Context, leases []*Lease, desired *Partition, expiresAt time.Time) (
	available *Lease, sp SortedPartitions, expiredLeases []*Lease) {
	now := clock.Now(ctx)
	builder := NewSortedPartitionsBuilder(desired)
	for _, l := range leases {
		if l.ExpiresAt.Before(now) {
			expiredLeases = append(expiredLeases, l)
			continue
		}
		for _, part := range l.Parts {
			if builder.IsEmpty() {
				break
			}
			p, err := PartitionFromString(part)
			if err != nil {
				// Should not happen in prod, unless the format has changed.
				// Log but proceed as if the lease didn't exist, it's still correct if 2
				// workers process overlapping range.
				logging.Errorf(ctx, "failed to parse a lease: %s", err)
				continue
			}
			builder.Exclude(&p)
		}
	}
	if !builder.IsEmpty() {
		sp = builder.Result()
		available = &Lease{
			ExpiresAt: expiresAt.Round(time.Millisecond).UTC(),
			Parts:     make([]string, len(sp)),
		}
		for i, p := range sp {
			available.Parts[i] = p.String()
		}
	}
	return
}
