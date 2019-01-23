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

// createInstancesAsync schedules task queue tasks to create each GCE instance.
func createInstancesAsync(c context.Context) error {
	var keys []*datastore.Key
	q := datastore.NewQuery(model.VMKind).Eq("url", "")
	if err := datastore.GetAll(c, q, &keys); err != nil {
		return errors.Annotate(err, "failed to fetch VMs").Err()
	}
	t := make([]*tq.Task, len(keys))
	for i, k := range keys {
		id := k.StringID()
		t[i] = &tq.Task{
			Payload: &tasks.CreateInstance{
				Id: id,
			},
		}
		logging.Debugf(c, "found VM %q", id)
	}
	if err := getDispatcher(c).AddTask(c, t...); err != nil {
		return errors.Annotate(err, "failed to schedule tasks").Err()
	}
	return nil
}

// deleteVMsAsync schedules task queue tasks to delete each VM.
func deleteVMsAsync(c context.Context) error {
	var keys []*datastore.Key
	q := datastore.NewQuery(model.VMKind).Eq("drained", true)
	if err := datastore.GetAll(c, q, &keys); err != nil {
		return errors.Annotate(err, "failed to fetch VMs").Err()
	}
	t := make([]*tq.Task, len(keys))
	for i, k := range keys {
		id := k.StringID()
		t[i] = &tq.Task{
			Payload: &tasks.DeleteVM{
				Id: id,
			},
		}
		logging.Debugf(c, "found VM %q", id)
	}
	if err := getDispatcher(c).AddTask(c, t...); err != nil {
		return errors.Annotate(err, "failed to schedule tasks").Err()
	}
	return nil
}

// manageBotsAsync schedules task queue tasks to manage each Swarming bot.
func manageBotsAsync(c context.Context) error {
	var keys []*datastore.Key
	// TODO(smut): Fetch in batches.
	q := datastore.NewQuery(model.VMKind).Gt("url", "")
	if err := datastore.GetAll(c, q, &keys); err != nil {
		return errors.Annotate(err, "failed to fetch VMs").Err()
	}
	t := make([]*tq.Task, len(keys))
	for i, k := range keys {
		id := k.StringID()
		t[i] = &tq.Task{
			Payload: &tasks.ManageBot{
				Id: id,
			},
		}
		logging.Debugf(c, "found VM %q", id)
	}
	if err := getDispatcher(c).AddTask(c, t...); err != nil {
		return errors.Annotate(err, "failed to schedule tasks").Err()
	}
	return nil
}

// processConfigsAsync schedules task queue tasks to process each config.
func processConfigsAsync(c context.Context) error {
	var keys []*datastore.Key
	if err := datastore.GetAll(c, datastore.NewQuery(model.ConfigKind), &keys); err != nil {
		return errors.Annotate(err, "failed to fetch configs").Err()
	}
	t := make([]*tq.Task, len(keys))
	for i, k := range keys {
		id := k.StringID()
		t[i] = &tq.Task{
			Payload: &tasks.ProcessConfig{
				Id: id,
			},
		}
		logging.Debugf(c, "found config %q", id)
	}
	if err := getDispatcher(c).AddTask(c, t...); err != nil {
		return errors.Annotate(err, "failed to schedule tasks").Err()
	}
	return nil
}

// reportQuotasAsync schedules task queue tasks to report quota in each project.
func reportQuotasAsync(c context.Context) error {
	var keys []*datastore.Key
	if err := datastore.GetAll(c, datastore.NewQuery(model.ProjectKind), &keys); err != nil {
		return errors.Annotate(err, "failed to fetch project").Err()
	}
	t := make([]*tq.Task, len(keys))
	for i, k := range keys {
		id := k.StringID()
		t[i] = &tq.Task{
			Payload: &tasks.ReportQuota{
				Id: id,
			},
		}
		logging.Debugf(c, "found project %q", id)
	}
	if err := getDispatcher(c).AddTask(c, t...); err != nil {
		return errors.Annotate(err, "failed to schedule tasks").Err()
	}
	return nil
}
