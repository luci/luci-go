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

// Package tasks contains task queue implementations.
package tasks

import (
	"context"
	"encoding/json"
	"fmt"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/tq"

	taskdefs "go.chromium.org/luci/buildbucket/appengine/tasks/defs"

	// Enable datastore transactional tasks support.
	_ "go.chromium.org/luci/server/tq/txn/datastore"
)

// rejectionHandler returns a tq.Handler which rejects the given task.
// Used by tasks which are handled in Python.
// TODO(crbug/1042991): Remove once all handlers are implemented in Go.
func rejectionHandler(tq string) tq.Handler {
	return func(ctx context.Context, payload proto.Message) error {
		logging.Errorf(ctx, "tried to handle %s: %q", tq, payload)
		return errors.Reason("handler called").Err()
	}
}

func init() {
	tq.RegisterTaskClass(tq.TaskClass{
		ID: "cancel-swarming-task",
		Custom: func(ctx context.Context, m proto.Message) (*tq.CustomPayload, error) {
			task := m.(*taskdefs.CancelSwarmingTask)
			body, err := json.Marshal(map[string]interface{}{
				"hostname": task.Hostname,
				"task_id":  task.TaskId,
				"realm":    task.Realm,
			})
			if err != nil {
				return nil, errors.Annotate(err, "error marshaling payload").Err()
			}
			return &tq.CustomPayload{
				Body:        body,
				Method:      "POST",
				RelativeURI: fmt.Sprintf("/internal/task/buildbucket/cancel_swarming_task/%s/%s", task.Hostname, task.TaskId),
			}, nil
		},
		Handler:   rejectionHandler("cancel-swarming-task"),
		Kind:      tq.Transactional,
		Prototype: (*taskdefs.CancelSwarmingTask)(nil),
		Queue:     "backend-default",
	})

	tq.RegisterTaskClass(tq.TaskClass{
		ID: "create-swarming-task",
		Custom: func(ctx context.Context, m proto.Message) (*tq.CustomPayload, error) {
			task := m.(*taskdefs.CreateSwarmingTask)
			body, err := json.Marshal(map[string]interface{}{
				"generation": 0,
				"id":         task.BuildId,
			})
			if err != nil {
				return nil, errors.Annotate(err, "error marshaling payload").Err()
			}
			return &tq.CustomPayload{
				Body:        body,
				Method:      "POST",
				RelativeURI: fmt.Sprintf("/internal/task/swarming/sync-build/%d", task.BuildId),
			}, nil
		},
		Handler:   rejectionHandler("create-swarming-task"),
		Kind:      tq.Transactional,
		Prototype: (*taskdefs.CreateSwarmingTask)(nil),
		Queue:     "swarming-build-create",
	})

	tq.RegisterTaskClass(tq.TaskClass{
		ID: "export-bigquery",
		Custom: func(ctx context.Context, m proto.Message) (*tq.CustomPayload, error) {
			task := m.(*taskdefs.ExportBigQuery)
			body, err := json.Marshal(map[string]interface{}{
				"id": task.BuildId,
			})
			if err != nil {
				return nil, errors.Annotate(err, "error marshaling payload").Err()
			}
			return &tq.CustomPayload{
				Body:        body,
				Method:      "POST",
				RelativeURI: fmt.Sprintf("/internal/task/bq/export/%d", task.BuildId),
			}, nil
		},
		Handler:   rejectionHandler("export-bigquery"),
		Kind:      tq.Transactional,
		Prototype: (*taskdefs.ExportBigQuery)(nil),
		Queue:     "backend-default",
	})

	tq.RegisterTaskClass(tq.TaskClass{
		ID: "finalize-resultdb",
		Custom: func(ctx context.Context, m proto.Message) (*tq.CustomPayload, error) {
			task := m.(*taskdefs.FinalizeResultDB)
			body, err := json.Marshal(map[string]interface{}{
				"id": task.BuildId,
			})
			if err != nil {
				return nil, errors.Annotate(err, "error marshaling payload").Err()
			}
			return &tq.CustomPayload{
				Body:        body,
				Method:      "POST",
				RelativeURI: fmt.Sprintf("/internal/task/resultdb/finalize/%d", task.BuildId),
			}, nil
		},
		Handler:   rejectionHandler("finalize-resultdb"),
		Kind:      tq.Transactional,
		Prototype: (*taskdefs.FinalizeResultDB)(nil),
		Queue:     "backend-default",
	})

	tq.RegisterTaskClass(tq.TaskClass{
		ID: "notify-pubsub",
		Custom: func(ctx context.Context, m proto.Message) (*tq.CustomPayload, error) {
			task := m.(*taskdefs.NotifyPubSub)
			mode := "global"
			if task.Callback {
				mode = "callback"
			}
			body, err := json.Marshal(map[string]interface{}{
				"id":   task.BuildId,
				"mode": mode,
			})
			if err != nil {
				return nil, errors.Annotate(err, "error marshaling payload").Err()
			}
			return &tq.CustomPayload{
				Body:        body,
				Method:      "POST",
				RelativeURI: fmt.Sprintf("/internal/task/buildbucket/notify/%d", task.BuildId),
			}, nil
		},
		Handler:   rejectionHandler("notify-pubsub"),
		Kind:      tq.Transactional,
		Prototype: (*taskdefs.NotifyPubSub)(nil),
		Queue:     "backend-default",
	})

	tq.RegisterTaskClass(tq.TaskClass{
		ID:        "cancel-build",
		Kind:      tq.Transactional,
		Prototype: (*taskdefs.CancelBuildTask)(nil),
		Queue:     "backend-go-default",
		Handler: func(ctx context.Context, payload proto.Message) error {
			t := payload.(*taskdefs.CancelBuildTask)
			_, err := Cancel(ctx, t.BuildId)
			return err
		},
	})
}

// CancelSwarmingTask enqueues a task queue task to cancel the given Swarming
// task.
func CancelSwarmingTask(ctx context.Context, task *taskdefs.CancelSwarmingTask) error {
	switch {
	case task.GetHostname() == "":
		return errors.Reason("hostname is required").Err()
	case task.TaskId == "":
		return errors.Reason("task_id is required").Err()
	}
	return tq.AddTask(ctx, &tq.Task{
		Payload: task,
	})
}

// CreateSwarmingTask enqueues a task queue task to create a Swarming task from
// the given build.
func CreateSwarmingTask(ctx context.Context, task *taskdefs.CreateSwarmingTask) error {
	if task.GetBuildId() == 0 {
		return errors.Reason("build_id is required").Err()
	}
	return tq.AddTask(ctx, &tq.Task{
		Payload: task,
	})
}

// ExportBigQuery enqueues a task queue task to export the given build to
// BigQuery.
func ExportBigQuery(ctx context.Context, task *taskdefs.ExportBigQuery) error {
	if task.GetBuildId() == 0 {
		return errors.Reason("build_id is required").Err()
	}
	return tq.AddTask(ctx, &tq.Task{
		Payload: task,
	})
}

// FinalizeResultDB enqueues a task queue task to finalize the invocation for
// the given build in ResultDB.
func FinalizeResultDB(ctx context.Context, task *taskdefs.FinalizeResultDB) error {
	if task.GetBuildId() == 0 {
		return errors.Reason("build_id is required").Err()
	}
	return tq.AddTask(ctx, &tq.Task{
		Payload: task,
	})
}
