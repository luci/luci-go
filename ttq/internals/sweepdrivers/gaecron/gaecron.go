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

// Package gaecron implements Sweeper on Google AppEngine
// using Cloud Tasks and builtin AppEngine cron.
package gaecron

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"sync"
	"time"

	taskspb "google.golang.org/genproto/googleapis/cloud/tasks/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/appengine/gaemiddleware"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/ttq/internals"
	dslessor "go.chromium.org/luci/ttq/internals/lessors/datastore"
	"go.chromium.org/luci/ttq/internals/partition"
	"go.chromium.org/luci/ttq/internals/reminder"
)

// NewSweeper creates a new Sweeper.
//
// You must:
//
//   * configure cron to hit "`pathPrefix`/cron" every minute. For example,
//     add the following to cron.yaml:
//         - description: ttq sweeping
//           url: /internal/ttq/cron
//           schedule: every 1 minutes
//
//   * configure a task queue with at least 10 QPS. For example,
//     add the following to queue.yaml:
//         - name: ttq-sweep
//           rate: 10/s
//     Note: you may also re-use existing queue or create a new one via Cloud
//     Tasks API or gcloud command line. See also
//     https://cloud.google.com/tasks/docs/reference/rest/v2/projects.locations.queues/create.
//
// Arguments:
//     r is the router to install handler into.
//     mw is the middleware to use. Empty middleware is fine. On classic
//         AppEngine, standard.Base is recommended (see
//         https://godoc.org/go.chromium.org/luci/appengine/gaemiddleware/standard#Base).
//     pathPrefix will be reserved for the sweeper use via provided router.
//         "/internal/ttq" is recommended.
//         Must start with "/".
//     queue must be a full Cloud Tasks Queue name. Format is
//         `projects/PROJECT_ID/locations/LOCATION_ID/queues/QUEUE_ID`.
//         Hint: lookup the full name of the queue given its short name from
//         queue.yaml with this command:
//             $ gcloud --project <project> tasks queues describe short-queue-name
//     tasksClient should be Cloud Tasks Client in production, see
//         https://godoc.org/cloud.google.com/go/cloudtasks/apiv2#Client
//
// For ease of debugging, if pathPrefix and queue don't meet some necessary
// requirements, panics. However, no panic doesn't mean queue name is correct
// or that it actually exists.
func NewSweeper(impl *internals.Impl, pathPrefix, queue string, tasksClient internals.TasksClient) *Sweeper {
	switch err := internals.ValidateQueueName(queue); {
	case err != nil:
		panic(err)
	case !strings.HasPrefix(pathPrefix, "/"):
		panic(fmt.Sprintf("pathPrefix %q doesn't start with /", pathPrefix))
	case !strings.HasSuffix(pathPrefix, "/"):
		pathPrefix += "/"
	}
	return &Sweeper{
		pathPrefix:     pathPrefix,
		queue:          queue,
		tasksClient:    internals.UnborkTasksClient(tasksClient),
		impl:           impl,
		ppBatchSize:    50,
		ppBatchWorkers: 8,
	}
}

// Sweeper drives the sweeping process using AppEngine cronjob and Cloud Tasks
// executed on AppEngine.
type Sweeper struct {
	pathPrefix  string
	queue       string
	tasksClient internals.TasksClient
	impl        *internals.Impl
	lessor      dslessor.Lessor

	ppBatchSize    int // max reminders to PostProcess in a batch.
	ppBatchWorkers int // max workers to PostProcess batches per task.
}

func (s *Sweeper) InstallRoutes(r *router.Router, mw router.MiddlewareChain) {
	shortQueue := s.queue[strings.LastIndex(s.queue, "/")+1:]
	r.GET(s.pathPrefix+"/cron", mw.Extend(gaemiddleware.RequireCron), s.handleCron)
	r.POST(s.pathPrefix+"/work/*debugname", mw.Extend(gaemiddleware.RequireTaskQueue(shortQueue)),
		s.handleTask)
}

func (s *Sweeper) handleCron(rctx *router.Context) {
	scanItems := s.impl.SweepAll()
	err := s.createTasks(rctx.Context, scanItems)
	internals.StatusOffError(rctx, err)
}

func (s *Sweeper) handleTask(rctx *router.Context) {
	var err error
	var body []byte
	defer func() { internals.StatusOffError(rctx, err) }()

	body, err = ioutil.ReadAll(rctx.Request.Body)
	if err != nil {
		err = errors.Annotate(err, "failed to read request body").Err()
		return
	}
	scan := internals.ScanItem{}
	if err = json.Unmarshal(body, &scan); err != nil {
		err = errors.Annotate(err, "failed to unmarshal request body").Err()
		return
	}
	err = s.execTask(rctx.Context, scan)
}

func (s *Sweeper) createTask(ctx context.Context, scan internals.ScanItem) error {
	body, err := json.MarshalIndent(scan, "", "  ")
	if err != nil {
		return errors.Annotate(err, "failed to marshal %v", scan).Err()
	}
	req := &taskspb.CreateTaskRequest{
		Parent: s.queue,
		Task: &taskspb.Task{
			MessageType: &taskspb.Task_AppEngineHttpRequest{
				AppEngineHttpRequest: &taskspb.AppEngineHttpRequest{
					HttpMethod: taskspb.HttpMethod_POST,
					RelativeUri: fmt.Sprintf("%s/work/%d-%d-%s", s.pathPrefix,
						scan.Shard, scan.Level, scan.Partition),
					Body: body,
				},
			},
		},
	}
	_, err = s.tasksClient.CreateTask(ctx, req)
	switch code := status.Code(err); code {
	case codes.OK, codes.AlreadyExists:
		return nil
	case codes.InvalidArgument:
		return errors.Annotate(err, "invalid create Cloud Task request: %v", req).Err()
	default:
		return errors.Annotate(err, "failed to create Cloud Task").Tag(transient.Tag).Err()
	}
}

// execTask executes one scan task and any follow postProcessing as a result.
// If more scans required as a followup, schedules them via push tasks.
func (s *Sweeper) execTask(ctx context.Context, scan internals.ScanItem) error {
	// Ensure there is time to postProcess Reminders produced by scan.
	scanTimeout := time.Minute
	if d, ok := ctx.Deadline(); ok {
		scanTimeout = d.Sub(clock.Now(ctx)) / 5
	}
	scanCtx, cancel := clock.WithTimeout(ctx, scanTimeout)
	defer cancel()
	moreScans, scanResult, err := s.impl.Scan(scanCtx, scan)
	if err != nil {
		return err
	}

	wg := sync.WaitGroup{}
	wg.Add(2)
	lerr := errors.NewLazyMultiError(2)
	go func() {
		lerr.Assign(0, s.createTasks(ctx, moreScans))
		wg.Done()
	}()
	go func() {
		lerr.Assign(1, s.postProcessAll(ctx, scanResult))
		wg.Done()
	}()
	wg.Wait()
	return lerr.Get() // nil if no errors
}

func (s *Sweeper) createTasks(ctx context.Context, scans []internals.ScanItem) error {
	// 16 is the default number of shards.
	return parallel.WorkPool(16, func(workChan chan<- func() error) {
		for _, scan := range scans {
			scan := scan
			workChan <- func() error { return s.createTask(ctx, scan) }
		}
	})
}

func (s *Sweeper) postProcessAll(ctx context.Context, scanResult internals.ScanResult) error {
	l := len(scanResult.Reminders)
	if l == 0 {
		return nil
	}
	desired, err := partition.SpanInclusive(scanResult.Reminders[0].Id, scanResult.Reminders[l-1].Id)
	if err != nil {
		return errors.Annotate(err, "invalid Reminder Id(s)").Err()
	}
	lockID := fmt.Sprintf("%d", scanResult.Shard)
	var errProcess error
	leaseErr := s.lessor.WithLease(ctx, lockID, desired, time.Minute,
		func(leaseCtx context.Context, leased partition.SortedPartitions) {
			reminders := internals.OnlyLeased(scanResult.Reminders, leased, 16)
			errProcess = s.postProcessWithLease(leaseCtx, reminders)
		})
	switch {
	case leaseErr != nil:
		return errors.Annotate(leaseErr, "failed to acquire lease").Err()
	case errProcess != nil:
		return errors.Annotate(errProcess, "failed to postProcess all reminders").Err()
	default:
		return nil
	}
}

func (s *Sweeper) postProcessWithLease(ctx context.Context, reminders []*reminder.Reminder) error {
	return parallel.WorkPool(s.ppBatchWorkers, func(workChan chan<- func() error) {
		for {
			var batch []*reminder.Reminder
			switch l := len(reminders); {
			case l == 0:
				return
			case l < s.ppBatchSize:
				batch, reminders = reminders, nil
			default:
				batch, reminders = reminders[:s.ppBatchSize], reminders[s.ppBatchSize:]
			}
			workChan <- func() error { return s.impl.PostProcessBatch(ctx, batch) }
		}
	})
}
