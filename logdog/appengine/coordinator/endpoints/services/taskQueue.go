// Copyright 2019 The LUCI Authors.
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

package services

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/gae/service/datastore"
	ds "go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/gae/service/taskqueue"
	"go.chromium.org/luci/grpc/grpcutil"

	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
	"go.chromium.org/luci/logdog/appengine/coordinator"
)

// baseArchiveQueueName is the taskqueue queue name which contains Archive tasks.
const baseArchiveQueueName = "archiveTasks"

// RawArchiveQueueName returns the raw name for the queueNumber'th queue used
// for ArchiveTasks.
func RawArchiveQueueName(queueNumber int32) string {
	if queueNumber == 0 {
		// logdog queues didn't used to be sharded, so keep baseArchiveQueueName for
		// queue 0.
		return baseArchiveQueueName
	}
	return fmt.Sprintf("%s-%d", baseArchiveQueueName, queueNumber)
}

var (
	metricCreateTask = metric.NewCounter(
		"logdog/archival/create_task",
		"Number of created tasks from the archive queue, as seen by the coordinator",
		nil,
		field.String("project"),
		field.Int("queueNumber"))
	metricLeaseTask = metric.NewCounter(
		"logdog/archival/lease_task",
		"Number of leased tasks from the archive queue, as seen by the coordinator",
		nil,
		field.Int("numRetries"),
		field.String("project"),
		field.Int("queueNumber"))
	metricDeleteTask = metric.NewCounter(
		"logdog/archival/delete_task",
		"Number of delete task request for the archive queue, as seen by the coordinator",
		nil,
		field.String("project"),
		field.Int("queueNumber"))
)

var (
	taskqueueLeaseRetry = func() retry.Iterator {
		// very simple retry scheme; we only have 60s to service the entire
		// request so we can't let this grow too large.
		return &retry.Limited{
			Delay:   time.Second,
			Retries: 3,
		}
	}
)

// taskArchival tasks an archival of a stream with the given delay.
func (s *server) taskArchival(c context.Context, state *coordinator.LogStreamState, realm string, delay time.Duration) error {
	// Now task the archival.
	state.Updated = clock.Now(c).UTC()
	state.ArchivalKey = []byte{'1'} // Use a fake key just to signal that we've tasked the archival.
	if err := ds.Put(c, state); err != nil {
		log.Fields{
			log.ErrorKey: err,
		}.Errorf(c, "Failed to Put() LogStream.")
		return status.Error(codes.Internal, "internal server error")
	}

	project := string(coordinator.ProjectFromNamespace(state.Parent.Namespace()))
	id := string(state.ID())
	t, err := tqTask(&logdog.ArchiveTask{Project: project, Id: id, Realm: realm})
	if err != nil {
		log.WithError(err).Errorf(c, "could not create archival task")
		return status.Error(codes.Internal, "internal server error")
	}
	t.Delay = delay
	queueName, queueNumber := s.getNextArchiveQueueName(c)

	if err := taskqueue.Add(c, queueName, t); err != nil {
		log.WithError(err).Errorf(c, "could not task archival")
		return status.Error(codes.Internal, "internal server error")
	}

	metricCreateTask.Add(c, 1, project, queueNumber)
	return nil
}

// tqTask returns a taskqueue task for an archive task.
func tqTask(task *logdog.ArchiveTask) (*taskqueue.Task, error) {
	if task.TaskName != "" {
		panic("task.TaskName is set while generating task")
	}

	payload, err := proto.Marshal(task)
	return &taskqueue.Task{
		Payload: payload,
		Method:  "PULL",
	}, err
}

// tqTaskLeased returns a taskqueue queue name and task from a previously-leased
// archive task.
func tqTaskLeased(task *logdog.ArchiveTask) (queueNumber int32, t *taskqueue.Task, err error) {
	if task.TaskName == "" {
		panic("task.TaskName is unset while deleting task")
	}

	t = &taskqueue.Task{
		Method: "PULL",
	}

	toks := strings.Split(task.TaskName, ":")
	switch len(toks) {
	case 1:
		// legacy task: pre-sharding
		queueNumber = 0
		t.Name = task.TaskName

	case 2:
		var queueNumberInt int

		if queueNumberInt, err = strconv.Atoi(toks[0]); err != nil {
			err = errors.Annotate(err, "parsing TaskName %q", task.TaskName).Err()
			return
		}

		queueNumber = int32(queueNumberInt)
		t.Name = toks[1]

	default:
		err = errors.Reason("unknown TaskName format %q", task.TaskName).Err()
	}

	return
}

// archiveTask creates a archiveTask proto from a taskqueue task.
func archiveTask(task *taskqueue.Task, queueNumber int32) (*logdog.ArchiveTask, error) {
	result := logdog.ArchiveTask{}
	err := proto.Unmarshal(task.Payload, &result)
	// NOTE: TaskName here is the LogDog "task name" and is the composite of the
	// queue number we pulled from plus `task.Name` which is the auto-generated
	// task id assigned by the taskqueue service.
	result.TaskName = fmt.Sprintf("%d:%s", queueNumber, task.Name)
	return &result, err
}

// checkArchived is a best-effort check to see if a task is archived.
// If this fails, an error is logged, and it returns false.
func isArchived(c context.Context, task *logdog.ArchiveTask) bool {
	var err error
	defer func() {
		if err != nil {
			logging.WithError(err).Errorf(c, "while checking if %s/%s is archived", task.Project, task.Id)
		}
	}()
	if c, err = info.Namespace(c, coordinator.ProjectNamespace(task.Project)); err != nil {
		return false
	}
	state := (&coordinator.LogStream{ID: coordinator.HashID(task.Id)}).State(c)
	if err = datastore.Get(c, state); err != nil {
		return false
	}
	return state.ArchivalState() == coordinator.ArchivedComplete
}

// LeaseArchiveTasks leases archive tasks to the requestor from a pull queue.
func (s *server) LeaseArchiveTasks(c context.Context, req *logdog.LeaseRequest) (*logdog.LeaseResponse, error) {
	if err := req.LeaseTime.CheckValid(); err != nil {
		return nil, err
	}
	duration := req.LeaseTime.AsDuration()

	var tasks []*taskqueue.Task
	logging.Infof(c, "got request to lease %d tasks for %s", req.MaxTasks, req.GetLeaseTime())

	queueName, queueNumber := s.getNextArchiveQueueName(c)
	logging.Infof(c, "picked queue %q", queueName)

	err := retry.Retry(c, taskqueueLeaseRetry,
		func() error {
			var err error

			tasks, err = taskqueue.Lease(c, int(req.MaxTasks), queueName, duration)
			// TODO(iannucci): There probably should be a better API for this error
			// detection stuff, but since taskqueue is deprecated, I don't think it's
			// worth the effort.
			ann := errors.Annotate(err, "leasing archive tasks")
			if err != nil && strings.Contains(err.Error(), "TRANSIENT_ERROR") {
				ann.Tag(
					grpcutil.ResourceExhaustedTag, // for HTTP response code 429.
					transient.Tag,                 // for retry.Retry.
				)
			}
			return ann.Err()
		},
		retry.LogCallback(c, "taskqueue.Lease"))
	if err != nil {
		return nil, err
	}

	logging.Infof(c, "got %d raw tasks from taskqueue", len(tasks))

	var archiveTasksL sync.Mutex
	archiveTasks := make([]*logdog.ArchiveTask, 0, len(tasks))

	parallel.WorkPool(8, func(ch chan<- func() error) {
		for _, task := range tasks {
			at, err := archiveTask(task, queueNumber)
			if err != nil {
				// Ignore malformed name errors, just log them.
				logging.WithError(err).Errorf(c, "while leasing task")
				continue
			}

			ch <- func() error {
				// Optimization: Delete the task if it's already archived.
				if isArchived(c, at) {
					if err := taskqueue.Delete(c, queueName, task); err != nil {
						logging.WithError(err).Errorf(c, "failed to delete %s/%s (%s)", at.Project, at.Id, task.Name)
					}
					metricDeleteTask.Add(c, 1, at.Project, queueNumber)
				} else {
					// BUG(crbug.com/1323896): This log is here to diagnosis of some weird
					// metrics we were seeing; Metrics reported retry counts of 70+, but
					// we weren't able to actually see which tasks those could be.
					if task.RetryCount > 50 {
						logging.Warningf(
							c, "crbug.com/1323896: High retry task encountered: project:%s, queue#:%d, retries:%d, taskName:%s",
							at.Project, queueNumber, task.RetryCount, task.Name)
					}

					archiveTasksL.Lock()
					defer archiveTasksL.Unlock()
					archiveTasks = append(archiveTasks, at)
					metricLeaseTask.Add(c, 1, task.RetryCount, at.Project, queueNumber)
				}
				return nil
			}
		}
	})

	logging.Infof(c, "Leasing %d tasks", len(archiveTasks))
	return &logdog.LeaseResponse{Tasks: archiveTasks}, nil
}

// DeleteArchiveTasks deletes archive tasks from the task queue.
// Errors are logged but ignored.
func (s *server) DeleteArchiveTasks(c context.Context, req *logdog.DeleteRequest) (*emptypb.Empty, error) {
	// Although we only ever issue archival tasks from a single queue, this RPC
	// doesn't stipulate that all tasks must belong to the same queue.
	tasksPerQueue := map[int32][]*taskqueue.Task{}
	for _, at := range req.Tasks {
		queueNumber, tqTask, err := tqTaskLeased(at)
		if err != nil {
			return nil, err
		}

		metricDeleteTask.Add(c, 1, at.Project, queueNumber)
		tasksPerQueue[queueNumber] = append(tasksPerQueue[queueNumber], tqTask)
	}

	return &emptypb.Empty{}, parallel.WorkPool(8, func(ch chan<- func() error) {
		for queueNumber, tasks := range tasksPerQueue {
			queueName := RawArchiveQueueName(queueNumber)

			ch <- func() error {
				logging.Infof(c, "Deleting %d tasks from %q", len(tasks), queueName)
				if err := taskqueue.Delete(c, queueName, tasks...); err != nil {
					logging.WithError(err).Errorf(c, "while deleting tasks\n%#v", tasks)
				}
				return nil
			}
		}
	})
}
