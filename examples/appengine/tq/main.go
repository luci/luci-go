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

// Demo of server/tq module on Appengine.
//
// It can also run locally, but it needs real Cloud Datastore, so you'll need
// to provide some Cloud Project name. Note that it will create real entities
// there which may interfere with the production server/tq deployment in this
// project, so use some experimental Cloud Project for this:
//
//	$ go run . -cloud-project your-experimental-project
//	$ curl http://127.0.0.1:8800/count-down/10
//	<observe logs>
//	$ curl http://127.0.0.1:8800/internal/tasks/c/sweep
//	<observe logs>
//
// To test this for real, deploy the GAE app:
//
//	$ gae.py upload -A your-experimental-project --switch
//	$ curl https://<your-experimental-project>.appspot.com/count-down/10
//	<observe logs>
//
// Again, be careful with what Cloud Project you are updating.
package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/examples/appengine/tq/taskspb"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/tq"

	// Enable datastore transactional tasks support.
	_ "go.chromium.org/luci/server/tq/txn/datastore"
)

// ExampleEntity is just some test entity to update in a transaction.
type ExampleEntity struct {
	_kind      string    `gae:"$kind,ExampleEntity"`
	ID         int64     `gae:"$id"`
	LastUpdate time.Time `gae:",noindex"`
}

func init() {
	// RegisterTaskClass tells the TQ module how to serialize, route and execute
	// a task of a particular proto type (*taskspb.CountDownTask in this case).
	//
	// It can be called any time before the serving loop (e.g. in an init,
	// in main, in server.Main callback, etc). init() is the preferred place.
	tq.RegisterTaskClass(tq.TaskClass{
		// This is a stable ID that identifies this particular kind of tasks.
		// Changing it will essentially "break" all inflight tasks.
		ID: "count-down-task",

		// This is used for deserialization and also for discovery of what ID to use
		// when submitting tasks. Changing it is safe as long as the JSONPB
		// representation of in-flight tasks still matches the new proto.
		Prototype: (*taskspb.CountDownTask)(nil),

		// This controls how AddTask calls behave with respect to transactions.
		// FollowsContext means "enqueue transactionally if the context is
		// transactional, or non-transactionally otherwise". Other possibilities are
		// Transactional (always require a transaction) and NonTransactional
		// (fail if called from a transaction).
		Kind: tq.FollowsContext,

		// What Cloud Tasks queue to use for these tasks. See queue.yaml.
		Queue: "countdown-tasks",

		// Handler will be called to handle a previously submitted task. It can also
		// be attached later (perhaps even in from a different package) via
		// AttachHandler.
		Handler: func(ctx context.Context, payload proto.Message) error {
			task := payload.(*taskspb.CountDownTask)

			logging.Infof(ctx, "Got %d", task.Number)
			if task.Number <= 0 {
				return nil // stop counting
			}

			return datastore.RunInTransaction(ctx, func(ctx context.Context) error {
				// Update some entity.
				err := datastore.Put(ctx, &ExampleEntity{
					ID:         task.Number,
					LastUpdate: clock.Now(ctx).UTC(),
				})
				if err != nil {
					return err
				}
				// And transactionally enqueue the next task. Note if you need to submit
				// more tasks, it is fine to call multiple Enqueue (or AddTask) in
				// parallel.
				return EnqueueCountDown(ctx, task.Number-1)
			}, nil)
		},
	})
}

// EnqueueCountDown enqueues a count down task.
func EnqueueCountDown(ctx context.Context, num int64) error {
	return tq.AddTask(ctx, &tq.Task{
		// The body of the task. Also identifies what TaskClass to use.
		Payload: &taskspb.CountDownTask{Number: num},
		// Title appears in logs and URLs, useful for debugging.
		Title: fmt.Sprintf("count-%d", num),
		// How long to wait before executing this task. Not super precise.
		Delay: 100 * time.Millisecond,
	})
}

func main() {
	modules := []module.Module{
		gaeemulation.NewModuleFromFlags(), // to use Cloud Datastore
		tq.NewModuleFromFlags(),           // to transactionally submit Cloud Tasks
	}

	server.Main(nil, modules, func(srv *server.Server) error {
		srv.Routes.GET("/count-down/:From", nil, func(c *router.Context) {
			num, err := strconv.ParseInt(c.Params.ByName("From"), 10, 32)
			if err != nil {
				http.Error(c.Writer, "Not a number", http.StatusBadRequest)
				return
			}
			// Kick off the chain by enqueuing the starting task.
			if err = EnqueueCountDown(c.Request.Context(), num); err != nil {
				http.Error(c.Writer, err.Error(), http.StatusInternalServerError)
			} else {
				c.Writer.Write([]byte("OK\n"))
			}
		})
		return nil
	})
}
