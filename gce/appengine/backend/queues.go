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
	"fmt"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/gce/api/config/v1"
	"go.chromium.org/luci/gce/api/tasks/v1"
)

// ensureQueue is the name of the ensure task handler queue.
const ensureQueue = "ensure-vm"

// ensure creates or updates a given VM.
func ensure(c context.Context, payload proto.Message) error {
	task, ok := payload.(*tasks.Ensure)
	switch {
	case !ok:
		return errors.Reason("unexpected payload %q", payload).Err()
	case task.GetId() == "":
		return errors.Reason("ID is required").Err()
	}
	logging.Debugf(c, "ensuring %q", task.Id)
	// TODO(smut): Create or update this VM.
	return nil
}

// expandQueue is the name of the expand task handler queue.
const expandQueue = "expand-config"

// expand creates task queue tasks to process each VM in the given VMs block.
func expand(c context.Context, payload proto.Message) error {
	task, ok := payload.(*tasks.Expand)
	switch {
	case !ok:
		return errors.Reason("unexpected payload %q", payload).Err()
	case task.GetId() == "":
		return errors.Reason("ID is required").Err()
	}
	logging.Debugf(c, "expanding %q", task.Id)
	vms, err := getServer(c).GetVMs(c, &config.GetVMsRequest{Id: task.Id})
	if err != nil {
		return errors.Annotate(err, "failed to get VMs block").Err()
	}
	logging.Debugf(c, "found %d VMs", vms.Amount)
	t := make([]*tq.Task, vms.Amount)
	for i := int32(0); i < vms.Amount; i++ {
		t[i] = &tq.Task{
			Payload: &tasks.Ensure{
				Id:         fmt.Sprintf("%s-%d", task.Id, i),
				Attributes: vms,
			},
		}
	}
	if err := getDispatcher(c).AddTask(c, t...); err != nil {
		return errors.Annotate(err, "failed to schedule tasks").Err()
	}
	return nil
}
