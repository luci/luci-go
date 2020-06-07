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

// Binary demo contains minimal demo for 'ttq' package.
package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	"github.com/google/uuid"
	taskspb "google.golang.org/genproto/googleapis/cloud/tasks/v2"

	ds "go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/ttq"
	"go.chromium.org/luci/ttq/datastore"
)

const (
	qTTQ         = "projects/tandrii-test-crbug-1080423/locations/us-central1/queues/ttq-internal"
	qUserHappy   = "projects/tandrii-test-crbug-1080423/locations/us-central1/queues/user-q"
	qUserUnhappy = "projects/tandrii-test-crbug-1080423/locations/us-central1/queues/user-q-2"
	qLoadHappy   = "projects/tandrii-test-crbug-1080423/locations/us-central1/queues/load-q"
	qLoadUnhappy = "projects/tandrii-test-crbug-1080423/locations/us-central1/queues/load-q-2"
)

var metricTaskCreated = metric.NewCounter(
	ttq.MPrefix+"user/created",
	"Number of user tasks triggered (right after successful transaction)",
	nil,
	field.String("kind"),
	field.String("db"),
)
var metricTaskExecuted = metric.NewCounter(
	ttq.MPrefix+"user/executed",
	"Number of user tasks executed",
	nil,
	field.String("kind"),
	field.String("db"),
)

type Mother struct {
	_kind string `gae:"$kind,demo.Mother"`
}

type Child struct {
	_kind string `gae:"$kind,demo.Child"`
}

func main() {
	modules := []module.Module{
		gaeemulation.NewModuleFromFlags(),
	}
	server.Main(nil, modules, func(srv *server.Server) error {
		ctc, err := cloudtasks.NewClient(srv.Context)
		if err != nil {
			panic(err)
		}
		host := "https://" + srv.Options.CloudProject + ".appspot.com"
		d := ttq.New(qTTQ, host+"/handle/ttq", ctc, datastore.New())

		srv.Routes.POST("/hit-me/:opt", router.MiddlewareChain{}, func(ctx *router.Context) {
			metricTaskExecuted.Add(ctx.Context, 1, ctx.Params.ByName("opt"), "datastore")
			fmt.Fprintf(ctx.Writer, "Done")
		})

		srv.Routes.GET("/trans/:opt", router.MiddlewareChain{}, func(rctx *router.Context) {
			opt := rctx.Params.ByName("opt")
			if opt == "" {
				opt = "happy"
			}
			targetQ := qUserHappy
			if opt != "happy" {
				targetQ = qUserUnhappy
			}
			var pp ttq.PostProcess
			u, err := uuid.NewRandom()
			if err != nil {
				panic(err)
			}
			err = ds.RunInTransaction(rctx.Context, func(ctx context.Context) (err error) {
				pp, err = d.Enqueue(ctx, &taskspb.Task{
					Name: fmt.Sprintf("%s/tasks/%s", targetQ, u.String()),
					MessageType: &taskspb.Task_HttpRequest{HttpRequest: &taskspb.HttpRequest{
						HttpMethod: taskspb.HttpMethod_POST,
						Url:        host + "/hit-me/" + opt,
						Body:       []byte(opt),
						// TODO
						//AuthorizationHeader: &HttpRequest_OidcToken{OidcToken: &taskspb.OidcToken{
						//	ServiceAccountEmail: "TODO",
						//	Audience:            "",
						//}},
					}},
				})
				if err != nil {
					return err
				}
				if opt == "err" {
					return errors.Reason("as asked").Err()
				}
				return nil
			}, nil)
			if err != nil {
				logging.Errorf(rctx.Context, "error: %s", err)
				fmt.Fprintf(rctx.Writer, "error: %s", err)
				return
			}
			metricTaskCreated.Add(rctx.Context, 1, opt, "datastore")
			if opt == "unhappy" {
				fmt.Fprintf(rctx.Writer, "unhappy")
				return
			}
			fmt.Fprintf(rctx.Writer, "post-processing")
			pp(rctx.Context)
			fmt.Fprintf(rctx.Writer, "done")
		})

		srv.Routes.GET("/load/:count/:kind", router.MiddlewareChain{}, func(ctx *router.Context) {
			m, err := strconv.Atoi(ctx.Params.ByName("count"))
			if err != nil {
				panic(err)
			}
			kind := ctx.Params.ByName("kind")
			qTarget := qLoadHappy
			if kind != "happy" {
				qTarget = qLoadUnhappy
			}
			err = parallel.FanOutIn(func(workChan chan<- func() error) {
				for i := 0; i < m; i++ {
					workChan <- func() error {
						return createTaskWorkaround(ctx.Context, ctc, &taskspb.CreateTaskRequest{
							Parent: qTarget,
							Task: &taskspb.Task{
								MessageType: &taskspb.Task_HttpRequest{HttpRequest: &taskspb.HttpRequest{
									HttpMethod: taskspb.HttpMethod_GET,
									Url:        host + "/trans/" + kind,
									// TODO
									//AuthorizationHeader: &HttpRequest_OidcToken{OidcToken: &taskspb.OidcToken{
									//	ServiceAccountEmail: "TODO",
									//	Audience:            "",
									//}},
								}},
							},
						})
					}
				}
			})
			logging.Debugf(ctx.Context, "dispatched %d with err %s", m, err)
			fmt.Fprintf(ctx.Writer, "error: %s", err)
		})

		srv.Routes.GET("/sweep", router.MiddlewareChain{}, func(ctx *router.Context) {
			if err := d.Sweep(ctx.Context); err != nil {
				panic(err)
			}
			fmt.Fprintf(ctx.Writer, "swept")
		})

		srv.Routes.GET("/handle/ttq/shard/:part", router.MiddlewareChain{}, func(rctx *router.Context) {
			if err := d.SweepShard(rctx.Context, rctx.Params.ByName("part")); err != nil {
				panic(err)
			}
			fmt.Fprintf(rctx.Writer, "swept")
		})

		return nil
	})
}

func createTaskWorkaround(ctx context.Context, ctc *cloudtasks.Client, req *taskspb.CreateTaskRequest) error {
	// WORKAROUND(https://github.com/googleapis/google-cloud-go/issues/1577): if
	// the passed context deadline is larger than 30s, the CreateTask call fails
	// with InvalidArgument "The request deadline is ... The deadline cannot be
	// more than 30s in the future." So, give it 20s.
	ctx, cancel := context.WithTimeout(ctx, time.Second*20)
	defer cancel()
	_, err := ctc.CreateTask(ctx, req)
	return err
}
