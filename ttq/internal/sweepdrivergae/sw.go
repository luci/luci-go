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

// Package sweepdrivergae implements SweepDriver on Google AppEngine
// using Cloud Tasks and builtin AppEngine cron.
package sweepdrivergae

import (
	"context"
	"fmt"
	"io/ioutil"
	"strings"

	taskspb "google.golang.org/genproto/googleapis/cloud/tasks/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"

	"go.chromium.org/luci/appengine/gaemiddleware"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/ttq/internal"
)

// Factory is a SweepDriverFactory for AppEngine.
//
// You must:
//
//   * configure cron to hit "`pathPrefix`/cron" every minute. For example,
//     add the following to cron.yaml:
//         - description: ttq sweeping
//           url: /internal/ttq/cron
// 	        schedule: every 1 minutes
//
//   * configure a task queue with at least 10 QPS. For example,
//     add the following to queue.yaml:
//         - name: ttq-sweep
//           rate: 10/s
//     Note: you may also re-use existing queue or create a new one via Cloud
//     Tasks API or gcloud command line. See also
//     https://cloud.google.com/tasks/docs/reference/rest/v2/projects.locations.queues/create.
//
//   * configure automatic access restriction on "`pathPrefix`/". For example,
//     add the following to the "handlers:" section of app.yaml:
//         - url: /internal/ttq/.*
//           login: admin
//
// Arguments:
//		 r is the router to install handler into.
//     mw is the middleware to use. Empty middleware is fine. On classic
//				 AppEngine, standard.Base is recommended (see
//				 https://godoc.org/go.chromium.org/luci/appengine/gaemiddleware/standard#Base).
//		 pathPrefix will be reserved for the sweeper use via provided router.
//         "/internal/ttq" is recommended.
//         Must start with "/".
//		 queue must be a full Cloud Tasks Queue name. Format is
//				 `projects/PROJECT_ID/locations/LOCATION_ID/queues/QUEUE_ID`.
//				 Hint: lookup the full name of the queue given its short name from
//				 queue.yaml with this command:
//						 $ gcloud --project <project> tasks queues describe short-queue-name
//     tasksClient should be Cloud Tasks Client in production, see
//         https://godoc.org/cloud.google.com/go/cloudtasks/apiv2#Client
//
// For ease of debugging, if pathPrefix and queue don't meet *necessary*
// requirements, panics. However, no panic doesn't mean queue name is correct or
// actually exists.
func Factory(r *router.Router, mw router.MiddlewareChain,
	pathPrefix, queue string, tasksClient internal.TasksClient) internal.SweepDriverFactory {
	switch err := internal.ValidateQueueName(queue); {
	case err != nil:
		panic(err)
	case !strings.HasPrefix(pathPrefix, "/"):
		panic(fmt.Sprintf("pathPrefix %q doesn't start with /", pathPrefix))
	case !strings.HasSuffix(pathPrefix, "/"):
		pathPrefix += "/"
	}
	shortQueue := queue[strings.LastIndex(queue, "/")+1:]
	return func(impl internal.Sweepable) internal.SweepDriver {
		s := &sweepDriver{
			pathPrefix:  pathPrefix,
			queue:       queue,
			tasksClient: internal.UnborkTasksClient(tasksClient),
			impl:        impl}
		r.GET(pathPrefix+"/cron", mw.Extend(gaemiddleware.RequireCron), s.handleCron)
		r.POST(pathPrefix+"/work/*debugname", mw.Extend(gaemiddleware.RequireTaskQueue(shortQueue)),
			s.handleTask)
		return s
	}
}

type sweepDriver struct {
	pathPrefix  string
	queue       string
	tasksClient internal.TasksClient
	impl        internal.Sweepable
}

var _ internal.SweepDriver = (*sweepDriver)(nil)

func (s *sweepDriver) handleCron(rctx *router.Context) {
	err := s.impl.SweepCron(rctx.Context)
	internal.StatusOffError(rctx, err)
}

func (s *sweepDriver) handleTask(rctx *router.Context) {
	var err error
	var body []byte
	defer func() { internal.StatusOffError(rctx, err) }()

	body, err = ioutil.ReadAll(rctx.Request.Body)
	if err != nil {
		err = errors.Annotate(err, "failed to read request body").Err()
		return
	}
	w := internal.SweepWorkItem{}
	if err = protojson.Unmarshal(body, &w); err != nil {
		err = errors.Annotate(err, "failed to unmarshal request body").Err()
		return
	}
	err = s.impl.ExecSweepWorkItem(rctx.Context, &w)
}

// AsyncSweep schedules a SweepWorkItem for later execution.
// Implements internal.SweepDriver interface.
func (s *sweepDriver) AsyncSweep(ctx context.Context, w *internal.SweepWorkItem, dedupeKey string) error {
	body, err := protojson.Marshal(w)
	if err != nil {
		return errors.Annotate(err, "failed to marshal work item %v", w).Err()
	}
	name := ""
	if dedupeKey != "" {
		name = s.queue + "/tasks/" + dedupeKey
	}
	req := &taskspb.CreateTaskRequest{
		Parent: s.queue,
		Task: &taskspb.Task{
			Name:         name,
			ScheduleTime: w.Eta,
			MessageType: &taskspb.Task_AppEngineHttpRequest{
				AppEngineHttpRequest: &taskspb.AppEngineHttpRequest{
					HttpMethod: taskspb.HttpMethod_POST,
					RelativeUri: fmt.Sprintf("%s/work/%d-%d-%s", s.pathPrefix,
						w.Shard, w.Level, w.Partition),
					Body: body,
				},
			},
		},
	}
	_, err = s.tasksClient.CreateTask(ctx, req)
	switch code := status.Code(err); code {
	case codes.OK:
		return nil
	case codes.AlreadyExists:
		return nil
	case codes.InvalidArgument:
		return errors.Annotate(err, "invalid create Cloud Task request: %v", req).Err()
	default:
		return errors.Annotate(err, "failed to create Cloud Task").Tag(transient.Tag).Err()
	}
}
