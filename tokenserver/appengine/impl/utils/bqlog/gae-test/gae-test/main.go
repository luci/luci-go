// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package gaetest implements a sloppy sample app that tests 'bqlog' on GAE.
//
// It can be used to examine datastore performance and tweak bqlog parameters.
package gaetest

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"

	"golang.org/x/net/context"

	"github.com/luci/gae/service/info"
	"github.com/luci/luci-go/appengine/gaemiddleware"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/server/router"

	"github.com/luci/luci-go/tokenserver/appengine/impl/utils/bqlog"
)

var goatTeleportations = bqlog.Log{
	QueueName: "pull-queue",
	DatasetID: "testing",
	TableID:   "teleportations",
}

func goatTeleported(ctx context.Context, id int) error {
	return goatTeleportations.Insert(ctx, bqlog.Entry{
		Data: map[string]interface{}{
			"goat_id":     id,
			"instance_id": info.InstanceID(ctx),
			"time":        float64(clock.Now(ctx).UnixNano()) / 1e9,
		},
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
	basemw := gaemiddleware.BaseProd()

	gaemiddleware.InstallHandlers(r, basemw)

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
