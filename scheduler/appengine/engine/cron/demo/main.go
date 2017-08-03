// Copyright 2017 The LUCI Authors.
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

// Package demo shows how cron.Machines can be hosted with Datastore and TQ.
package demo

import (
	"net/http"
	"time"

	"golang.org/x/net/context"

	"github.com/golang/protobuf/proto"
	"github.com/luci/gae/service/datastore"

	"github.com/luci/luci-go/appengine/gaemiddleware"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/data/rand/mathrand"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/router"

	"github.com/luci/luci-go/appengine/tq"
	"github.com/luci/luci-go/scheduler/appengine/engine/cron"
	"github.com/luci/luci-go/scheduler/appengine/engine/internal"
	"github.com/luci/luci-go/scheduler/appengine/schedule"
)

var tasks = tq.Dispatcher{}

type CronState struct {
	_extra datastore.PropertyMap `gae:"-,extra"`

	ID    string     `gae:"$id"`
	State cron.State `gae:",noindex"`
}

func (s *CronState) schedule() *schedule.Schedule {
	parsed, err := schedule.Parse(s.ID, 0)
	if err != nil {
		panic(err)
	}
	return parsed
}

// evolve instantiates cron.Machine, calls the callback and submits emitted
// actions.
func evolve(c context.Context, id string, cb func(context.Context, *cron.Machine) error) error {
	err := datastore.RunInTransaction(c, func(c context.Context) error {
		entity := CronState{ID: id}
		if err := datastore.Get(c, &entity); err != nil && err != datastore.ErrNoSuchEntity {
			return err
		}

		machine := &cron.Machine{
			Now:      clock.Now(c),
			Schedule: entity.schedule(),
			Nonce:    func() int64 { return mathrand.Get(c).Int63() + 1 },
			State:    entity.State,
		}

		if err := cb(c, machine); err != nil {
			return err
		}

		for _, action := range machine.Actions {
			var task tq.Task
			switch a := action.(type) {
			case cron.TickLaterAction:
				logging.Infof(c, "Scheduling tick %d after %s", a.TickNonce, a.When.Sub(time.Now()))
				task = tq.Task{
					Payload: &internal.TickLaterTask{JobId: id, TickNonce: a.TickNonce},
					ETA:     a.When,
				}
			case cron.StartInvocationAction:
				task = tq.Task{
					Payload: &internal.StartInvocationTask{JobId: id},
					Delay:   time.Second, // give the transaction time to land
				}
			default:
				panic("unknown action type")
			}
			if err := tasks.AddTask(c, &task); err != nil {
				return err
			}
		}

		entity.State = machine.State
		return datastore.Put(c, &entity)
	}, nil)

	if err != nil {
		logging.Errorf(c, "FAIL - %s", err)
	}
	return err
}

func startJob(c context.Context, id string) error {
	return evolve(c, id, func(c context.Context, m *cron.Machine) error {
		// Forcefully restart the chain of tasks.
		m.Disable()
		m.Enable()
		return nil
	})
}

func handleTick(c context.Context, task proto.Message, execCount int) error {
	msg := task.(*internal.TickLaterTask)
	return evolve(c, msg.JobId, func(c context.Context, m *cron.Machine) error {
		return m.OnTimerTick(msg.TickNonce)
	})
}

func handleInvocation(c context.Context, task proto.Message, execCount int) error {
	msg := task.(*internal.StartInvocationTask)
	logging.Infof(c, "INVOCATION of job %q has finished!", msg.JobId)
	return evolve(c, msg.JobId, func(c context.Context, m *cron.Machine) error {
		m.RewindIfNecessary()
		return nil
	})
}

func init() {
	r := router.New()
	gaemiddleware.InstallHandlers(r)

	tasks.RegisterTask(&internal.TickLaterTask{}, handleTick, "default", nil)
	tasks.RegisterTask(&internal.StartInvocationTask{}, handleInvocation, "default", nil)
	tasks.InstallRoutes(r, gaemiddleware.BaseProd())

	// Kick-start a bunch of jobs by visiting:
	//
	// http://localhost:8080/start/with 10s interval
	// http://localhost:8080/start/with 5s interval
	// http://localhost:8080/start/0 * * * * * * *
	//
	// And the look at the logs.

	r.GET("/start/:JobID", gaemiddleware.BaseProd(), func(c *router.Context) {
		jobID := c.Params.ByName("JobID")
		if err := startJob(c.Context, jobID); err != nil {
			panic(err)
		}
	})

	http.DefaultServeMux.Handle("/", r)
}
