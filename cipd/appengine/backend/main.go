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

// Binary backend implements HTTP server that handles task queues and crons.
package main

import (
	"net/http"

	"google.golang.org/appengine"

	"go.chromium.org/luci/appengine/gaemiddleware"
	"go.chromium.org/luci/appengine/gaemiddleware/standard"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/cipd/appengine/impl"
	"go.chromium.org/luci/cipd/appengine/impl/model"
	"go.chromium.org/luci/cipd/appengine/impl/monitoring"
)

func main() {
	r := router.New()
	base := standard.Base()
	cron := base.Extend(gaemiddleware.RequireCron)

	standard.InstallHandlers(r)
	impl.TQ.InstallRoutes(r, base)

	r.GET("/internal/cron/bqlog/events-flush", cron,
		func(c *router.Context) {
			// FlushEventsToBQ logs errors inside. We also do not retry on errors.
			// It's fine to wait and flush on the next iteration.
			model.FlushEventsToBQ(c.Context)
			c.Writer.WriteHeader(http.StatusOK)
		},
	)
	r.GET("/internal/cron/import-config", cron,
		func(c *router.Context) {
			if err := monitoring.ImportConfig(c.Context); err != nil {
				errors.Log(c.Context, err)
				c.Writer.WriteHeader(http.StatusInternalServerError)
				return
			}
			c.Writer.WriteHeader(http.StatusOK)
		},
	)

	http.DefaultServeMux.Handle("/", r)
	appengine.Main()
}
