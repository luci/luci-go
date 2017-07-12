// Copyright 2016 The LUCI Authors.
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

package distributor

import (
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/dm/appengine/model"
	"github.com/luci/luci-go/tumble"
	"golang.org/x/net/context"
)

// NotifyExecution is used to finish an execution. Specifically it allows the
// appropriate distributor to HandleNotification, and then when that concludes,
// invokes DM's FinishExecution (see mutate.FinishExecution).
type NotifyExecution struct {
	CfgName      string
	Notification *Notification
}

// Root implements tumble.Mutation.
func (f *NotifyExecution) Root(c context.Context) *ds.Key {
	return model.ExecutionKeyFromID(c, f.Notification.ID)
}

// RollForward implements tumble.Mutation.
func (f *NotifyExecution) RollForward(c context.Context) (muts []tumble.Mutation, err error) {
	reg := GetRegistry(c)
	dist, _, err := reg.MakeDistributor(c, f.CfgName)
	if err != nil {
		logging.Fields{
			logging.ErrorKey: err,
			"cfg":            f.CfgName,
		}.Errorf(c, "Failed to make distributor")
		return
	}

	q := &model.Quest{ID: f.Notification.ID.Quest}
	if err := ds.Get(ds.WithoutTransaction(c), q); err != nil {
		return nil, errors.Annotate(err, "getting Quest").Err()
	}
	rslt, err := dist.HandleNotification(&q.Desc, f.Notification)
	if err != nil {
		// TODO(riannucci): check for transient/non-transient
		logging.Fields{
			logging.ErrorKey: err,
			"cfg":            f.CfgName,
		}.Errorf(c, "Failed to handle notification")
		return
	}
	if rslt != nil {
		return reg.FinishExecution(c, f.Notification.ID, rslt)
	}
	return
}

func init() {
	tumble.Register((*NotifyExecution)(nil))
}
