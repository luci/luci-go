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

package tq

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	taskspb "cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb"

	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/server/tq/tqtesting"
)

// TestingContext creates a scheduler that executes tasks through the given
// dispatcher (or Default one if nil) and puts it into the context as Submitter,
// so AddTask calls eventually submit tasks into this scheduler.
//
// The end result is that tasks submitted using such context end up in the
// returned Scheduler (allowing them to be examined), and when the Scheduler
// delivers them, they result in calls to corresponding handlers registered in
// the Dispatcher.
func TestingContext(ctx context.Context, d *Dispatcher) (context.Context, *tqtesting.Scheduler) {
	if d == nil {
		d = &Default
	}
	sched := &tqtesting.Scheduler{Executor: &directExecutor{d}}
	return UseSubmitter(ctx, sched), sched
}

// directExecutor implements tqtesting.Executor via handlePush.
type directExecutor struct {
	d *Dispatcher
}

func (e *directExecutor) Execute(ctx context.Context, t *tqtesting.Task, done func(retry bool)) {
	retry := false
	defer func() { done(retry) }()

	switch {
	case t.Message != nil && t.Task != nil:
		panic("Both Task and Message are set") // break tests loudly
	case t.Message != nil:
		// Execute PubSub message by marking it done immediately.
		logging.Debugf(ctx, "server/tq: sent a Pubsub message")
		return
	}

	var body []byte
	var headers map[string]string
	switch mt := t.Task.MessageType.(type) {
	case *taskspb.Task_HttpRequest:
		body = mt.HttpRequest.Body
		headers = mt.HttpRequest.Headers
	case *taskspb.Task_AppEngineHttpRequest:
		body = mt.AppEngineHttpRequest.Body
		headers = mt.AppEngineHttpRequest.Headers
	default:
		panic(fmt.Sprintf("Bad task, no payload: %q", t.Task))
	}

	hdr := make(http.Header, len(headers))
	for k, v := range headers {
		hdr.Set(k, v)
	}
	info := parseHeaders(hdr)

	// The direct executor doesn't emulate X-CloudTasks-* headers.
	info.ExecutionCount = t.Attempts - 1
	if index := strings.LastIndex(t.Name, "/tasks/"); index > 0 {
		info.TaskID = t.Name[index+len("/tasks/"):]
	}

	ctx = logging.SetField(ctx, fmt.Sprintf("TQ-%.8s", info.TaskID), info.ExecutionCount)
	err := e.d.handlePush(ctx, body, info)
	if err != nil {
		if !quietOnError.In(err) {
			logging.Errorf(ctx, "server/tq task error: %s", err)
		}
		retry = !Fatal.In(err) && !Ignore.In(err)
	}
}
