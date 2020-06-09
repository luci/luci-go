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
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	"cloud.google.com/go/spanner"
	"github.com/google/uuid"
	"google.golang.org/api/option"
	taskspb "google.golang.org/genproto/googleapis/cloud/tasks/v2"

	ds "go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/secrets"

	"go.chromium.org/luci/ttq"
	"go.chromium.org/luci/ttq/ttqds"
	"go.chromium.org/luci/ttq/ttqspanner"
)

const (
	qTTQds       = "projects/tandrii-test-crbug-1080423/locations/us-central1/queues/ttq-ds"
	qTTQsp       = "projects/tandrii-test-crbug-1080423/locations/us-central1/queues/ttq-sp"
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
var metricTaskLatency = metric.NewCumulativeDistribution(
	ttq.MPrefix+"user/latency",
	"Latency between inside of the transaction to execution",
	&types.MetricMetadata{Units: types.Milliseconds},
	distribution.DefaultBucketer,
	field.String("kind"),
	field.String("db"),
)

func main() {
	modules := []module.Module{
		gaeemulation.NewModuleFromFlags(),
		secrets.NewModuleFromFlags(),
	}
	server.Main(nil, modules, func(srv *server.Server) error {
		ctc, err := cloudtasks.NewClient(srv.Context)
		if err != nil {
			panic(err)
		}
		spc, err := mkSpannerClient(srv.Context)
		if err != nil {
			panic(err)
		}
		srv.RegisterCleanup(func(_ context.Context) { spc.Close() })
		spDB := ttqspanner.New(spc)
		host := "https://" + srv.Options.CloudProject + ".appspot.com"

		initTTQs := func() map[string]*ttq.TTQ {
			d := ttqds.New()
			ttqs := map[string]*ttq.TTQ{
				d.Kind():    ttq.New(qTTQds, host+"/ttq/"+d.Kind(), ctc, d),
				spDB.Kind(): ttq.New(qTTQsp, host+"/ttq/"+spDB.Kind(), ctc, spDB),
			}
			return ttqs
		}
		ttqs := initTTQs()

		validateDB := func(rctx *router.Context) string {
			db := rctx.Params.ByName("db")
			for kind, _ := range ttqs {
				if kind == db {
					return kind
				}
			}
			panic(errors.Reason("bad db %q", db))
		}

		validateKind := func(rctx *router.Context) string {
			kind := rctx.Params.ByName("kind")
			if kind == "happy" || kind == "unhappy" {
				return kind
			}
			panic(errors.Reason("bad kind %q", kind))
		}

		srv.Routes.POST("/hit/:db/:kind", router.MiddlewareChain{}, func(rctx *router.Context) {
			db := validateDB(rctx)
			kind := validateKind(rctx)
			metricTaskExecuted.Add(rctx.Context, 1, kind, db)
			if bs, err := ioutil.ReadAll(rctx.Request.Body); err == nil {
				var startedAt time.Time
				if err = startedAt.UnmarshalJSON(bs); err == nil {
					delay := clock.Now(rctx.Context).Sub(startedAt)
					metricTaskLatency.Add(rctx.Context, float64(delay.Milliseconds()), kind, db)
				}
			}
			retOK(rctx, "OK")
		})

		srv.Routes.GET("/trans/:db/:kind", router.MiddlewareChain{}, func(rctx *router.Context) {
			db := validateDB(rctx)
			opt := validateKind(rctx)
			targetQ := qUserHappy
			if opt != "happy" {
				targetQ = qUserUnhappy
			}
			var pp ttq.PostProcess
			u, err := uuid.NewRandom()
			if err != nil {
				retFail(rctx, err, "uuid.NewRandom")
				return
			}
			httpReq := &taskspb.HttpRequest{
				HttpMethod: taskspb.HttpMethod_POST,
				Url:        fmt.Sprintf("%s/hit/%s/%s", host, db, opt),
				// TODO
				//AuthorizationHeader: &HttpRequest_OidcToken{OidcToken: &taskspb.OidcToken{
				//	ServiceAccountEmail: "TODO",
				//	Audience:            "",
				//}},
			}
			task := &taskspb.Task{
				Name:        fmt.Sprintf("%s/tasks/%s", targetQ, u.String()),
				MessageType: &taskspb.Task_HttpRequest{HttpRequest: httpReq},
			}
			updateBodyTimestamp := func() {
				var err error
				httpReq.Body, err = clock.Now(rctx.Context).MarshalJSON()
				if err != nil {
					panic(err)
				}
			}
			switch db {
			case "datastore":
				err = ds.RunInTransaction(rctx.Context, func(ctx context.Context) (err error) {
					updateBodyTimestamp()
					pp, err = ttqs[db].Enqueue(ctx, task)
					return err
				}, nil)
			case "spanner":
				err = spDB.RunInTransaction(rctx.Context, func(ctx context.Context) (err error) {
					updateBodyTimestamp()
					pp, err = ttqs[db].Enqueue(ctx, task)
					return err
				})
			default:
				panic(nil)
			}
			// TODO spanner.
			if err != nil {
				retFail(rctx, err, "trans failed")
				return
			}
			metricTaskCreated.Add(rctx.Context, 1, opt, db)
			if opt == "happy" {
				pp(rctx.Context)
			}
			retOK(rctx, opt)
		})

		srv.Routes.GET("/load/:db/:kind/:count", router.MiddlewareChain{}, func(ctx *router.Context) {
			count, err := strconv.Atoi(ctx.Params.ByName("count"))
			if err != nil {
				retFail(ctx, err, "must be int, not %q", ctx.Params.ByName("count"))
				return
			}
			db := validateDB(ctx)
			kind := validateKind(ctx)
			qTarget := qLoadHappy
			if kind != "happy" {
				qTarget = qLoadUnhappy
			}
			err = parallel.FanOutIn(func(workChan chan<- func() error) {
				for i := 0; i < count; i++ {
					workChan <- func() error {
						return createTaskWorkaround(ctx.Context, ctc, &taskspb.CreateTaskRequest{
							Parent: qTarget,
							Task: &taskspb.Task{
								MessageType: &taskspb.Task_HttpRequest{HttpRequest: &taskspb.HttpRequest{
									HttpMethod: taskspb.HttpMethod_GET,
									Url:        fmt.Sprintf("%s/trans/%s/%s", host, db, kind),
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
			if err != nil {
				merr, _ := err.(errors.MultiError)
				retFail(ctx, err, "failed to load-trigger %d out of %d tasks", len(merr), count)
				return
			}
			retOK(ctx, fmt.Sprintf("load-triggered %d tasks", count))
		})

		srv.Routes.GET("/sweep/:db", router.MiddlewareChain{}, func(rctx *router.Context) {
			db := validateDB(rctx)
			if err := ttqs[db].Sweep(rctx.Context); err != nil {
				retFail(rctx, err, "Sweep failed")
				return
			}
			retOK(rctx, "OK")
		})

		srv.Routes.GET("/ttq/:db/shard/:part", router.MiddlewareChain{}, func(rctx *router.Context) {
			db := validateDB(rctx)
			part := rctx.Params.ByName("part")
			if err := ttqs[db].SweepShard(rctx.Context, part); err != nil {
				retFail(rctx, err, "SweepShard(%s) failed", part)
				return
			}
			retOK(rctx, "OK")
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

func retFail(c *router.Context, e error, msg string, args ...interface{}) {
	errors.Log(c.Context, e)
	code := 0
	switch {
	case e == nil:
		panic("nil")
	case transient.Tag.In(e):
		code = 500
	default:
		code = 202 // fatal error, don't need a redelivery
	}
	args = append(args, e)
	body := fmt.Sprintf(msg+" - %s", args...)
	logging.Errorf(c.Context, "HTTP %d: %s", code, body)
	http.Error(c.Writer, body, code)
}

func retOK(c *router.Context, msg string) {
	c.Writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	c.Writer.WriteHeader(200)
	fmt.Fprintln(c.Writer, msg)
}

func mkSpannerClient(ctx context.Context) (*spanner.Client, error) {
	// projects/PROJECT_ID/instances/INSTANCE_ID/databases/DATABASE_ID.
	dbFlag := "projects/tandrii-test-crbug-1080423/instances/span-test/databases/db"

	trackSessionHandles := true

	// A token source with Cloud scope.
	ts, err := auth.GetTokenSource(ctx, auth.AsSelf, auth.WithScopes(auth.CloudOAuthScopes...))
	if err != nil {
		return nil, errors.Annotate(err, "failed to get the token source").Err()
	}

	// Init a Spanner client.
	cfg := spanner.ClientConfig{
		SessionPoolConfig: spanner.SessionPoolConfig{
			TrackSessionHandles: trackSessionHandles,
		},
	}
	spannerClient, err := spanner.NewClientWithConfig(ctx, dbFlag, cfg, option.WithTokenSource(ts))
	if err != nil {
		return nil, err
	}

	// Run a "ping" query to verify the database exists and we can access it
	// before we actually serve any requests. On misconfiguration better to fail
	// early.
	iter := spannerClient.Single().Query(ctx, spanner.NewStatement("SELECT 1;"))
	if err := iter.Do(func(*spanner.Row) error { return nil }); err != nil {
		return nil, errors.Annotate(err, "failed to ping Spanner").Err()
	}
	return spannerClient, nil
}
