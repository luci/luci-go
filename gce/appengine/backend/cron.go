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

// processHandler handles HTTP requests to process each VMs block.
func processHandler(c *router.Context) {
	c.Writer.Header().Set("Content-Type", "text/plain")

	if err := process(c.Context); err != nil {
		errors.Log(c.Context, err)
		c.Writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	c.Writer.WriteHeader(http.StatusOK)
}

// process creates task queue tasks to expand each VMs block.
func process(c context.Context) error {
	var keys []*datastore.Key
	if err := datastore.GetAll(c, datastore.NewQuery(model.VMsKind), &keys); err != nil {
		return errors.Annotate(err, "failed to fetch VMs blocks").Err()
	}
	t := make([]*tq.Task, len(keys))
	for i, k := range keys {
		id := k.StringID()
		t[i] = &tq.Task{
			Payload: &tasks.Expansion{
				Id: id,
			},
		}
		logging.Debugf(c, "found VMs block %q", id)
	}
	if err := getDispatcher(c).AddTask(c, t...); err != nil {
		return errors.Annotate(err, "failed to schedule tasks").Err()
	}
	return nil
}
