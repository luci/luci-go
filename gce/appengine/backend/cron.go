// Copyright 2018 The LUCI Authors.
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

package backend

import (
	"context"
	"net/http"
	"reflect"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/gce/api/tasks/v1"
	"go.chromium.org/luci/gce/appengine/model"
)

// newHTTPHandler returns a router.Handler which invokes the given function.
func newHTTPHandler(f func(c context.Context) error) router.Handler {
	return func(c *router.Context) {
		c.Writer.Header().Set("Content-Type", "text/plain")

		if err := f(c.Context); err != nil {
			errors.Log(c.Context, err)
			c.Writer.WriteHeader(http.StatusInternalServerError)
			return
		}

		c.Writer.WriteHeader(http.StatusOK)
	}
}

// payloadFn is a function which receives an ID and returns a proto.Message to
// use as the Payload in a *tq.Task.
type payloadFn func(string) proto.Message

// payloadFactory returns a payloadFn which can be called to return a
// proto.Message to use as the Payload in a *tq.Task.
func payloadFactory(t tasks.Task) payloadFn {
	rt := reflect.TypeOf(t).Elem()
	return func(id string) proto.Message {
		p := reflect.New(rt)
		p.Elem().FieldByName("Id").SetString(id)
		return p.Interface().(proto.Message)
	}
}

// trigger triggers a task queue task for each key returned by the given query.
func trigger(c context.Context, t tasks.Task, q *datastore.Query) error {
	tasks := make([]*tq.Task, 0)
	newPayload := payloadFactory(t)
	addTask := func(k *datastore.Key) {
		tasks = append(tasks, &tq.Task{
			Payload: newPayload(k.StringID()),
		})
	}
	if err := datastore.Run(c, q, addTask); err != nil {
		return errors.Annotate(err, "failed to fetch keys").Err()
	}
	logging.Debugf(c, "scheduling %d tasks", len(tasks))
	if err := getDispatcher(c).AddTask(c, tasks...); err != nil {
		return errors.Annotate(err, "failed to schedule tasks").Err()
	}
	return nil
}

// countVMsAsync schedules task queue tasks to count VMs for each config.
func countVMsAsync(c context.Context) error {
	return trigger(c, &tasks.CountVMs{}, datastore.NewQuery(model.ConfigKind))
}

// createInstancesAsync schedules task queue tasks to create each GCE instance.
func createInstancesAsync(c context.Context) error {
	return trigger(c, &tasks.CreateInstance{}, datastore.NewQuery(model.VMKind).Eq("url", ""))
}

// drainVMsAsync schedules task queue tasks to drain each VM.
func drainVMsAsync(c context.Context) error {
	return trigger(c, &tasks.DrainVM{}, datastore.NewQuery(model.VMKind))
}

// expandConfigsAsync schedules task queue tasks to expand each config.
func expandConfigsAsync(c context.Context) error {
	return trigger(c, &tasks.ExpandConfig{}, datastore.NewQuery(model.ConfigKind))
}

// manageBotsAsync schedules task queue tasks to manage each Swarming bot.
func manageBotsAsync(c context.Context) error {
	return trigger(c, &tasks.ManageBot{}, datastore.NewQuery(model.VMKind).Gt("url", ""))
}

// reportQuotasAsync schedules task queue tasks to report quota in each project.
func reportQuotasAsync(c context.Context) error {
	return trigger(c, &tasks.ReportQuota{}, datastore.NewQuery(model.ProjectKind))
}
