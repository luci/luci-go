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
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/gce/api/config/v1"
	"go.chromium.org/luci/gce/api/tasks/v1"
	"go.chromium.org/luci/gce/appengine/model"
)

// newHTTPHandler returns a router.Handler which invokes the given function.
func newHTTPHandler(f func(c context.Context) error) router.Handler {
	return func(c *router.Context) {
		c.Writer.Header().Set("Content-Type", "text/plain")

		if err := f(c.Request.Context()); err != nil {
			errors.Log(c.Request.Context(), err)
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
	now := time.Now().Unix()
	return func(id string) proto.Message {
		p := reflect.New(rt)
		p.Elem().FieldByName("Id").SetString(id)
		// Some tasks may don't need to track the created time.
		if v := p.Elem().FieldByName("TriggeredUnixTime"); v.IsValid() {
			v.SetInt(now)
		}
		return p.Interface().(proto.Message)
	}
}

// trigger triggers a task queue task for each key returned by the given query.
func trigger(c context.Context, t tasks.Task, q *datastore.Query) error {
	tasks := make([]*tq.Task, 0)
	newPayload := payloadFactory(t)
	for k, err := range datastore.RunQuery[*datastore.Key](c, q).Results {
		if err != nil {
			return errors.Fmt("failed to fetch keys: %w", err)
		}
		tasks = append(tasks, &tq.Task{
			Payload: newPayload(k.StringID()),
		})
	}
	logging.Debugf(c, "scheduling %d tasks", len(tasks))
	for _, task := range tasks {
		if err := getDispatcher(c).AddTask(c, task); err != nil {
			return errors.Fmt("failed to schedule tasks: %w", err)
		}
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

// expandConfigsAsync schedules task queue tasks to expand each config.
func expandConfigsAsync(c context.Context) error {
	return trigger(c, &tasks.ExpandConfig{}, datastore.NewQuery(model.ConfigKind))
}

// manageBotsAsync schedules task queue tasks to manage each Swarming bot.
func manageBotsAsync(c context.Context) error {
	return trigger(c, &tasks.ManageBot{}, datastore.NewQuery(model.VMKind).Gt("url", ""))
}

// drainVMsAsync comapres the config table to the vm table and determines the VMs that can be
// drained. It schedules a drainVM task for each of those VMs
func drainVMsAsync(c context.Context) error {
	configMap := make(map[string]*config.Config)
	// Get all the configs in datastore
	qC := datastore.NewQuery("Config")
	for cfg, err := range datastore.RunQuery[*model.Config](c, qC).Results {
		if err != nil {
			return errors.Fmt("drain vms: failed to list Config: %w", err)
		}
		configMap[cfg.ID] = cfg.Config
	}
	logging.Debugf(c, "Drain vms: staring...")
	vmMap := make(map[string]*model.VM)
	qV := datastore.NewQuery("VM")
	for vm, err := range datastore.RunQuery[*model.VM](c, qV).Results {
		if err != nil {
			return errors.Fmt("drain vms: failed to list VMs: %w", err)
		}
		vmMap[vm.ID] = vm
	}
	/* Config dictate how many VMs can be online for any given prefix. Check if there are
	 * more bots assigned than required by the config and drain them.
	 */
	//TODO(anushruth): Delete VMs based on uptime instead of ID.
	var taskList []*tq.Task
	for id, vm := range vmMap {
		if vm.DUT != "" {
			// DUT is still present in config. Index is not available for VM
			// mapped to DUT due to different sequences of creation.
			duts := configMap[vm.Config].GetDuts()
			if _, ok := duts[vm.DUT]; ok {
				continue
			}
		} else if configMap[vm.Config].GetCurrentAmount() > vm.Index {
			continue
		}
		logging.Debugf(c, "Drain vms: schedule %s to be drained", id)
		taskList = append(taskList, &tq.Task{
			Payload: &tasks.DrainVM{
				Id: id,
			},
		})
	}
	for _, task := range taskList {
		if err := getDispatcher(c).AddTask(c, task); err != nil {
			return errors.Fmt("drain vms: failed to schedule tasks: %w", err)
		}
	}
	return nil
}

// auditInstances schedules an audit task for every project:zone combination
func auditInstances(c context.Context) error {
	proj := stringset.New(10)
	q := datastore.NewQuery(model.ConfigKind)
	for cfg, err := range datastore.RunQuery[*model.Config](c, q).Results {
		if err != nil {
			return errors.Fmt("failed to schedule audits: %w", err)
		}
		proj.Add(cfg.Config.GetAttributes().GetProject())
	}
	projects := proj.ToSlice()
	jobs := make([]*tq.Task, 0)
	srv := getCompute(c).Stable.Zones
	for _, proj := range projects {
		zoneList, err := srv.List(proj).Context(c).Do()
		if err != nil {
			logging.Errorf(c, "Failed to list zones for %s. %v", proj, err)
			continue
		}
		for _, zone := range zoneList.Items {
			jobs = append(jobs, &tq.Task{
				Payload: &tasks.AuditProject{
					Project: proj,
					Zone:    zone.Name,
				},
			})
		}
	}
	for _, job := range jobs {
		if err := getDispatcher(c).AddTask(c, job); err != nil {
			return errors.Fmt("audit instances: failed to schedule tasks: %w", err)
		}
	}
	return nil
}

// reportQuotasAsync schedules task queue tasks to report quota in each project.
func reportQuotasAsync(c context.Context) error {
	return trigger(c, &tasks.ReportQuota{}, datastore.NewQuery(model.ProjectKind))
}

// countTasks counts tasks for each queue.
func countTasks(c context.Context) error {
	getDispatcher(c).ReportMetrics(c)
	return nil
}

// dumpDatastoreSync dumps datastore to BigQuery.
// The function runs in sync, instead of async as other cron handlers, as we
// don't suppose it takes long to finish.
func dumpDatastoreSync(c context.Context) error {
	ds, err := newBQDataset(c)
	if err != nil {
		return errors.Fmt("dump datastore: %w", err)
	}
	if err := uploadToBQ(c, ds); err != nil {
		return errors.Fmt("dump datastore: %w", err)
	}
	return nil
}
