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
)

type Dispatcher struct {
	*tq.Dispatcher
}

// dspKey is the key to a *tq.Dispatcher in the context.
var dspKey = "dsp"

// getDispatcher returns the *Dispatcher installed in the current context.
// Can be installed by WithDispatcher. Panics if there isn't one.
func GetDispatcher(c context.Context) *Dispatcher {
	return c.Value(&dspKey).(*Dispatcher)
}

// WithDispatcher returns a new context with the given *Dispatcher installed.
// Can be retrieved by GetDispatcher.
func WithDispatcher(c context.Context, dsp *Dispatcher) context.Context {
	return context.WithValue(c, &dspKey, dsp)
}

// NewDispatcher returns a new *Dispatcher constructed from the given
// *tq.Dispatcher.
func NewDispatcher(dsp *tq.Dispatcher) *Dispatcher {
	dsp.RegisterTaskClass(tq.TaskClass{
		ID: "cancel-swarming-task",
		Custom: func(ctx context.Context, m proto.Message) (*tq.CustomPayload, error) {
			task := m.(*taskdefs.CancelSwarmingTask)
			body, err := json.Marshal(map[string]string{
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
		Handler: func(ctx context.Context, payload proto.Message) error {
			// These tasks are handled by Python, it's an error for Go to receive them.
			// TODO(crbug/1042991): Implement these task queues in Go.
			logging.Errorf(ctx, "tried to handle cancel-swarming-task: %q", payload)
			return errors.Reason("handler called").Err()
		},
		Kind:      tq.Transactional,
		Prototype: (*taskdefs.CancelSwarmingTask)(nil),
		Queue:     "backend-default",
	})
	return &Dispatcher{dsp}
}

// CancelSwarmingTask enqueues a task queue task to cancel the given Swarming task.
func (d *Dispatcher) CancelSwarmingTask(ctx context.Context, task *taskdefs.CancelSwarmingTask) error {
	switch {
	case task.GetHostname() == "":
		return errors.Reason("hostname is required").Err()
	case task.TaskId == "":
		return errors.Reason("task_id is required").Err()
	}
	return d.AddTask(ctx, &tq.Task{
		Payload: task,
	})
}
