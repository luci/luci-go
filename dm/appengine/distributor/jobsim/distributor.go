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

package jobsim

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"golang.org/x/net/context"

	"github.com/golang/protobuf/jsonpb"

	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/taskqueue"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/dm/api/distributor/jobsim"
	dm "go.chromium.org/luci/dm/api/service/v1"
	"go.chromium.org/luci/dm/appengine/distributor"
)

type jobsimDist struct {
	c   context.Context
	cfg *distributor.Config
}

func (j *jobsimDist) jsConfig() *jobsim.Config {
	return j.cfg.Content.(*jobsim.Config)
}

func (j *jobsimDist) parsePayload(payload string) (*jobsimExecution, error) {
	ret := &jobsimExecution{}
	err := jsonpb.UnmarshalString(payload, &ret.Calculation)
	return ret, err
}

func (j *jobsimDist) Run(desc *dm.Quest_Desc, exAuth *dm.Execution_Auth, prev *dm.JsonResult) (tok distributor.Token, _ time.Duration, err error) {
	// TODO(riannucci): Fix luci-gae so we can truly escape the transaction when
	// we build the jobsimDist instance. See luci/gae#23.
	c := ds.WithoutTransaction(j.c)

	logging.Fields{
		"eid": exAuth.Id,
	}.Infof(j.c, "jobsim: running new task")

	jtsk, err := j.parsePayload(desc.Parameters)
	if err != nil {
		return
	}
	jtsk.ExAuth = *exAuth
	jtsk.Status = jobsimRunnable
	if prev != nil {
		jtsk.StateOrReason = prev.Object
	}
	jtsk.CfgName = j.cfg.Name

	key := []*ds.Key{
		ds.MakeKey(c, ds.GetMetaDefault(ds.GetPLS(jtsk), "kind", ""), 0)}
	if err = ds.AllocateIDs(c, key); err != nil {
		return
	}

	// transactionally commit the job and a taskqueue task to execute it
	jtsk.ID = fmt.Sprintf("%s|%d", j.jsConfig().Pool, key[0].IntID())
	logging.Infof(j.c, "jobsim: entering transaction")
	err = ds.RunInTransaction(c, func(c context.Context) error {
		if err := ds.Get(c, jtsk); err == nil {
			return nil
		}
		logging.Infof(j.c, "jobsim: got jtsk: %s", err)

		if err := ds.Put(c, jtsk); err != nil {
			return err
		}
		logging.Infof(j.c, "jobsim: put jtsk")

		err := j.cfg.EnqueueTask(c, &taskqueue.Task{
			Payload: []byte(jtsk.ID),
		})
		if err != nil {
			logging.WithError(err).Errorf(j.c, "jobsim: got EnqueueTask error")
			return err
		}
		logging.Infof(j.c, "jobsim: EnqueueTask'd")
		return nil
	}, nil)

	tok = distributor.Token(jtsk.ID)
	return
}

func (j *jobsimDist) Cancel(_ *dm.Quest_Desc, tok distributor.Token) error {
	jtsk := &jobsimExecution{ID: string(tok)}

	cancelBody := func(c context.Context) (needWrite bool, err error) {
		if err = ds.Get(c, jtsk); err != nil {
			return
		}
		if jtsk.Status != jobsimRunnable {
			return
		}
		needWrite = true
		return
	}

	if needWrite, err := cancelBody(j.c); err != nil || !needWrite {
		return err
	}

	return ds.RunInTransaction(j.c, func(c context.Context) error {
		if needWrite, err := cancelBody(c); err != nil || !needWrite {
			return err
		}
		jtsk.Status = jobsimCancelled
		return ds.Put(c, jtsk)
	}, nil)
}

func (j *jobsimDist) GetStatus(_ *dm.Quest_Desc, tok distributor.Token) (*dm.Result, error) {
	jtsk, err := loadTask(j.c, string(tok))
	if err != nil {
		return nil, err
	}

	return getAttemptResult(jtsk.Status, jtsk.StateOrReason), nil
}

func (j *jobsimDist) InfoURL(tok distributor.Token) string {
	return fmt.Sprintf("jobsim://%s/ver/%s/tok/%s", j.cfg.Name, j.cfg.Version, tok)
}

func (j *jobsimDist) HandleNotification(_ *dm.Quest_Desc, note *distributor.Notification) (*dm.Result, error) {
	n := &notification{}
	err := json.Unmarshal(note.Data, n)
	if err != nil {
		return nil, err
	}

	return getAttemptResult(n.Status, n.StateOrReason), nil
}

func loadTask(c context.Context, rawTok string) (*jobsimExecution, error) {
	logging.Fields{
		"jobsimId": rawTok,
	}.Infof(c, "jobsim: loading task")

	jtsk := &jobsimExecution{ID: string(rawTok)}
	if err := ds.Get(c, jtsk); err != nil {
		logging.WithError(err).Errorf(c, "jobsim: failed to load task")
		return nil, err
	}

	logging.Fields{
		"eid": jtsk.ExAuth.Id,
	}.Infof(c, "jobsim: execution context")

	return jtsk, nil
}

func (j *jobsimDist) HandleTaskQueueTask(r *http.Request) (notes []*distributor.Notification, err error) {
	// this function is what the lifetime of a remote-execution distributor (like
	// swarming) would do when running a scheduled task.

	// body is a distributor.Token
	rawTok, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return
	}

	jtsk, err := loadTask(j.c, string(rawTok))
	if err != nil {
		return
	}

	if jtsk.Status != jobsimRunnable {
		logging.Infof(j.c, "jobsim: task is not scheduled")
		return
	}
	jtsk.Status = jobsimRunning
	if err := ds.Put(j.c, jtsk); err != nil {
		logging.WithError(err).Warningf(j.c, "jobsim: failed to update task to Execution_Running")
	}

	state := &state{}
	if err = state.fromPersistentState(jtsk.StateOrReason); err != nil {
		logging.Fields{
			logging.ErrorKey: err,
			"state":          jtsk.StateOrReason,
		}.Errorf(j.c, "jobsim: unable to inflate PersistentState")
		return
	}

	// this is the part that the swarming client/recipe would do.
	err = runJob(j.c, j.cfg.DMHost, state, &jtsk.Calculation, &jtsk.ExAuth, jtsk.CfgName)
	if err != nil {
		jtsk.Status = jobsimFailed
		jtsk.StateOrReason = err.Error()
	} else {
		jtsk.Status = jobsimFinished
		jtsk.StateOrReason = state.toPersistentState()
	}

	err = retry.Retry(j.c, retry.Default, func() error {
		return ds.Put(j.c, jtsk)
	}, func(err error, wait time.Duration) {
		logging.Fields{
			logging.ErrorKey: err,
			"wait":           wait,
		}.Warningf(j.c, "jobsim: failed to put task")
	})
	if err != nil {
		logging.WithError(err).Errorf(j.c, "jobsim: FATAL: failed to put task")
		return
	}

	notes = append(notes, &distributor.Notification{
		ID:   jtsk.ExAuth.Id,
		Data: (&notification{jtsk.Status, state.toPersistentState()}).toJSON(),
	})
	return
}

func (j *jobsimDist) Validate(payload string) error {
	_, err := j.parsePayload(payload)
	return err
}

// AddFactory adds this distributor implementation into the distributor
// Registry.
func AddFactory(m distributor.FactoryMap) {
	m[(*jobsim.Config)(nil)] = func(c context.Context, cfg *distributor.Config) (distributor.D, error) {
		return &jobsimDist{c, cfg}, nil
	}
}
