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

// Package gaetest implements a sloppy sample app that tests 'bqlog' on GAE.
//
// It can be used to examine datastore performance and tweak bqlog parameters.
package gaetest

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"cloud.google.com/go/bigquery"

	"go.chromium.org/gae/service/info"
	"go.chromium.org/luci/appengine/gaemiddleware/standard"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/appengine/bqlog"
)

var goatTeleportations = bqlog.Log{
	QueueName: "pull-queue",
	DatasetID: "testing",
	TableID:   "teleportations",
}

type goatTeleportationEvent struct {
	GoatID     int
	InstanceID string
	Time       time.Time
}

// Save is part of bigquery.ValueSaver interface.
func (e *goatTeleportationEvent) Save() (map[string]bigquery.Value, string, error) {
	return map[string]bigquery.Value{
		"goat_id":     e.GoatID,
		"instance_id": e.InstanceID,
		"time":        float64(e.Time.UnixNano()) / 1e9,
	}, "", nil
}

func goatTeleported(ctx context.Context, id int) error {
	return goatTeleportations.Insert(ctx, &goatTeleportationEvent{
		GoatID:     id,
		InstanceID: info.InstanceID(ctx),
		Time:       clock.Now(ctx),
	})
}

var (
	globalID   = 0
	globalLock sync.Mutex
)

func getNextID() int {
	globalLock.Lock()
	defer globalLock.Unlock()
	globalID++
	return globalID
}

func init() {
	r := router.New()
	basemw := standard.Base()

	standard.InstallHandlers(r)

	r.GET("/generate/:Count", basemw, func(c *router.Context) {
		count, err := strconv.Atoi(c.Params.ByName("Count"))
		if err != nil {
			panic(err)
		}
		ctx := c.Context

		const parallel = 100
		count = (count / parallel) * parallel

		start := clock.Now(ctx)
		defer func() {
			fmt.Fprintf(c.Writer, "Generated %d events in %s", count, clock.Now(ctx).Sub(start))
		}()

		wg := sync.WaitGroup{}
		for idx := 0; idx < parallel; idx++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				for i := 0; i < count/parallel; i++ {
					goatTeleported(ctx, getNextID())
				}
			}(idx)
		}
		wg.Wait()
	})

	r.GET("/flush", basemw, func(c *router.Context) {
		ctx := c.Context
		start := clock.Now(ctx)
		flushed, err := goatTeleportations.Flush(ctx)
		fmt.Fprintf(c.Writer, "Flushed %d rows in %s\n", flushed, clock.Now(ctx).Sub(start))
		if err != nil {
			fmt.Fprintf(c.Writer, "Error: %s", err)
		}
	})

	http.DefaultServeMux.Handle("/", r)
}
