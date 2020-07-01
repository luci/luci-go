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

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	"google.golang.org/appengine"
	taskspb "google.golang.org/genproto/googleapis/cloud/tasks/v2"

	"go.chromium.org/luci/appengine/gaemiddleware"
	"go.chromium.org/luci/appengine/gaemiddleware/standard"
	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/luci_notify/config"
	"go.chromium.org/luci/luci_notify/notify"
)

var buildbucketPubSub = metric.NewCounter(
	"luci/notify/buildbucket-pubsub",
	"Number of received Buildbucket PubSub messages",
	nil,
	// "success", "transient-failure" or "permanent-failure"
	field.String("status"),
)

type ProdCloudTasksClient struct {
	ct         *cloudtasks.Client
	projectID  string
	locationID string
}

func (c ProdCloudTasksClient) CreateTask(ctx context.Context, queue string, task *taskspb.Task) (*taskspb.Task, error) {
	req := &taskspb.CreateTaskRequest{
		Parent: fmt.Sprintf("projects/%s/locations/%s/queues/%s", c.projectID, c.locationID, queue),
		Task:   task,
	}
	return c.ct.CreateTask(ctx, req)
}

func (c ProdCloudTasksClient) ProjectID() string {
	return c.projectID
}

func (c ProdCloudTasksClient) LocationID() string {
	return c.locationID
}

func newProdCloudTasksClient(ctx context.Context, projectID string, locationID string) (*ProdCloudTasksClient, error) {
	ct, err := cloudtasks.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	return &ProdCloudTasksClient{
		ct:         ct,
		projectID:  projectID,
		locationID: locationID,
	}, nil
}

func main() {
	r := router.New()
	standard.InstallHandlers(r)

	basemw := standard.Base()

	taskDispatcher := tq.Dispatcher{BaseURL: "/internal/tasks/"}
	notify.InitDispatcher(&taskDispatcher)
	taskDispatcher.InstallRoutes(r, basemw)

	// Cron endpoints.
	r.GET("/internal/cron/update-config", basemw.Extend(gaemiddleware.RequireCron), config.UpdateHandler)
	r.GET("/internal/cron/update-tree-status", basemw.Extend(gaemiddleware.RequireCron), notify.UpdateTreeStatus)

	// Pub/Sub endpoint.
	appID := os.Getenv("GAE_APPLICATION")
	// This can be extended to handle other region codes if needed.
	if len(appID) < 3 || !strings.HasPrefix(appID, "s~") {
		panic("Expected GAE_APPLICATION to be set and of the form s~appid.")
	}
	projectID := strings.TrimPrefix(appID, "s~")
	ct, err := newProdCloudTasksClient(context.Background(), projectID, "us-central1")
	if err != nil {
		panic(fmt.Sprintf("Unable to create cloud tasks client: %s", err.Error()))
	}

	defer ct.ct.Close()
	r.POST("/_ah/push-handlers/buildbucket", basemw,
		func(c *router.Context) {
			ctx, cancel := context.WithTimeout(c.Context, notify.PUBSUB_POST_REQUEST_TIMEOUT)
			defer cancel()
			c.Context = ctx

			status := ""
			switch err := notify.BuildbucketPubSubHandler(c, ct); {
			case transient.Tag.In(err) || appengine.IsTimeoutError(errors.Unwrap(err)):
				status = "transient-failure"
				logging.Errorf(ctx, "transient failure: %s", err)
				// Retry the message.
				c.Writer.WriteHeader(http.StatusInternalServerError)

			case err != nil:
				status = "permanent-failure"
				logging.Errorf(ctx, "permanent failure: %s", err)

			default:
				status = "success"
			}

			buildbucketPubSub.Add(ctx, 1, status)
		})

	http.Handle("/", r)
	appengine.Main()
}
