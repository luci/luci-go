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

	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/gae/service/taskqueue"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
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
	taskIDs := make([]string, 0, len(tasks))
	for _, task := range tasks {
		taskIDs = append(taskIDs, string(task.Name))
		leaseTask.Add(c, 1, task.RetryCount)
	}
	return &logdog.LeaseResponse{TaskIds: taskIDs}, nil
}

// DeleteArchiveTasks deletes archive tasks from the task queue.
func (b *server) DeleteArchiveTasks(c context.Context, req *logdog.DeleteRequest) (*empty.Empty, error) {
	deleteTask.Add(c, int64(len(req.TaskIds)))
	tasks := make([]*taskqueue.Task, 0, len(req.TaskIds))
	for _, id := range req.TaskIds {
		tasks = append(tasks, &taskqueue.Task{
			Name:   id,
			Method: "PULL",
		})
	}
	return nil, taskqueue.Delete(c, archiveQueueName, tasks...)
}
