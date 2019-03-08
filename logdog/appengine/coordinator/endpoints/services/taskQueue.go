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

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/info"
	"go.chromium.org/gae/service/taskqueue"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
	"go.chromium.org/luci/logdog/appengine/coordinator"
)

const archiveQueueName = "archiveTasks"

var (
	leaseTask = metric.NewCounter(
		"logdog/archival/lease_task",
		"Number of leased tasks from the archive queue, as seen by the coordinator",
		nil,
		field.Int("numRetries"))
	deleteTask = metric.NewCounter(
		"logdog/archival/delete_task",
		"Number of delete task request for the archive queue, as seen by the coordinator",
		nil)
)

// tqTask returns a taskqueue task for an archive task.
func tqTask(task *logdog.ArchiveTask) (*taskqueue.Task, error) {
	payload, err := proto.Marshal(task)
	return &taskqueue.Task{
		Name:    task.TaskName,
		Payload: payload,
		Method:  "PULL",
	}, err
}

// tqTaskLite returns a taskqueue task for an archive task without the payload.
func tqTaskLite(task *logdog.ArchiveTask) *taskqueue.Task {
	return &taskqueue.Task{
		Name:   task.TaskName,
		Method: "PULL",
	}
}

// archiveTask creates a archiveTask proto from a taskqueue task.
func archiveTask(task *taskqueue.Task) (*logdog.ArchiveTask, error) {
	result := logdog.ArchiveTask{}
	err := proto.Unmarshal(task.Payload, &result)
	result.TaskName = task.Name
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
	if c, err = info.Namespace(c, "luci."+task.Project); err != nil {
		return false
	}
	state := (&coordinator.LogStream{ID: coordinator.HashID(task.Id)}).State(c)
	if err = datastore.Get(c, state); err != nil {
		return false
	}
	return state.ArchivalState() == coordinator.ArchivedComplete
}

// LeaseArchiveTasks leases archive tasks to the requestor from a pull queue.
func (b *server) LeaseArchiveTasks(c context.Context, req *logdog.LeaseRequest) (*logdog.LeaseResponse, error) {
	duration, err := ptypes.Duration(req.LeaseTime)
	if err != nil {
		return nil, err
	}

	tasks, err := taskqueue.Lease(c, int(req.MaxTasks), archiveQueueName, duration)
	if err != nil {
		logging.WithError(err).Errorf(c, "could not lease %d tasks from queue", req.MaxTasks)
		return nil, err
	}
	archiveTasks := make([]*logdog.ArchiveTask, 0, len(tasks))
	for _, task := range tasks {
		at, err := archiveTask(task)
		if err != nil {
			// Ignore malformed name errors, just log them.
			logging.WithError(err).Errorf(c, "while leasing task")
			continue
		}
		// Optimization: Delete the task if it's already archived.
		if isArchived(c, at) {
			logging.Infof(c, "%s/%s is already archived, deleting.", at.Project, at.Id)
			if err := taskqueue.Delete(c, archiveQueueName, task); err != nil {
				logging.WithError(err).Errorf(c, "failed to delete %s/%s (%s)", at.Project, at.Id, task.Name)
			}
			continue
		}
		logging.Infof(c, "Leasing Project: %s, ID: %s, TaskName: %s", at.Project, at.Id, at.TaskName)
		archiveTasks = append(archiveTasks, at)
		leaseTask.Add(c, 1, task.RetryCount)
	}
	logging.Infof(c, "Leasing %d tasks", len(archiveTasks))
	return &logdog.LeaseResponse{Tasks: archiveTasks}, nil
}

// DeleteArchiveTasks deletes archive tasks from the task queue.
// Errors are logged but ignored.
func (b *server) DeleteArchiveTasks(c context.Context, req *logdog.DeleteRequest) (*empty.Empty, error) {
	deleteTask.Add(c, int64(len(req.Tasks)))
	tasks := make([]*taskqueue.Task, 0, len(req.Tasks))
	msg := fmt.Sprintf("Deleting %d tasks", len(req.Tasks))
	for _, at := range req.Tasks {
		msg += fmt.Sprintf("\nProject: %s, ID: %s, TaskName: %s", at.Project, at.Id, at.TaskName)
		tasks = append(tasks, tqTaskLite(at))
	}
	logging.Infof(c, msg)
	err := taskqueue.Delete(c, archiveQueueName, tasks...)
	if err != nil {
		logging.WithError(err).Errorf(c, "while deleting tasks\n%#v", tasks)
	}
	return &empty.Empty{}, nil
}
