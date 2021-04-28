// Copyright 2021 The LUCI Authors.
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

package cron

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/internal"
	"go.chromium.org/luci/server/router"
)

// Handler is called to handle a cron job invocation.
//
// Transient errors are transformed into HTTP 500 replies to Cloud Scheduler,
// which generally trigger an immediate retry. Returning a non-transient error
// results in a error-level logging message and HTTP 202 reply, which does not
// trigger a retry.
type Handler func(ctx context.Context) error

// Dispatcher routes requests from Cloud Scheduler to registered handlers.
type Dispatcher struct {
	// AuthorizedCallers is a list of service accounts Cloud Scheduler may use to
	// call cron HTTP endpoints.
	//
	// See https://cloud.google.com/scheduler/docs/http-target-auth for details.
	//
	// Can be empty on Appengine, since there calls are authenticated using
	// "X-Appengine-Cron" header.
	AuthorizedCallers []string

	// GAE is true when running on Appengine.
	//
	// It alters how incoming HTTP requests are authenticated.
	GAE bool

	// NoAuth can be used to disable authentication on HTTP endpoints.
	//
	// This is useful when running in development mode on localhost or in tests.
	NoAuth bool

	m sync.RWMutex
	h map[string]Handler
}

// RegisterHandler registers a callback called to handle a cron job invocation.
//
// The handler can be invoked via GET requests to "<serving-prefix>/<id>",
// (usually "/internal/cron/<id>"). This URL path should be used when
// configuring Cloud Scheduler jobs or in cron.yaml when running on Appengine.
//
// Panics if a handler with such ID is already registered.
func (d *Dispatcher) RegisterHandler(id string, h Handler) {
	d.m.Lock()
	defer d.m.Unlock()
	if d.h == nil {
		d.h = make(map[string]Handler, 1)
	}
	if _, ok := d.h[id]; ok {
		panic(fmt.Sprintf("cron handler with ID %q is already registered", id))
	}
	d.h[id] = h
}

// InstallCronRoutes installs routes that handle requests from Cloud Scheduler.
func (d *Dispatcher) InstallCronRoutes(r *router.Router, prefix string) {
	if prefix == "" {
		prefix = "/internal/cron/"
	} else if !strings.HasPrefix(prefix, "/") {
		panic("the prefix should start with /")
	}

	var mw router.MiddlewareChain
	if !d.NoAuth {
		header := ""
		if d.GAE {
			header = "X-Appengine-Cron"
		}
		mw = internal.CloudAuthMiddleware(d.AuthorizedCallers, header,
			func(ctx context.Context) {
				// TODO(vadimsh): Record in the rejection metric.
			},
		)
	}

	prefix = strings.TrimRight(prefix, "/") + "/*handler"
	r.GET(prefix, mw, func(c *router.Context) {
		id := strings.TrimPrefix(c.Params.ByName("handler"), "/")
		if err := d.executeHandlerByID(c.Context, id); err != nil {
			status := 0
			if transient.Tag.In(err) {
				err = errors.Annotate(err, "transient error in cron handler %q", id).Err()
				status = 500
			} else {
				err = errors.Annotate(err, "fatal error in cron handler %q", id).Err()
				status = 202
			}
			errors.Log(c.Context, err)
			http.Error(c.Writer, err.Error(), status)
		} else {
			c.Writer.Write([]byte("OK"))
		}
	})
}

// handlerIDs returns a list of registered handler IDs (in arbitrary order).
func (d *Dispatcher) handlerIDs() []string {
	d.m.RLock()
	defer d.m.RUnlock()
	ids := make([]string, 0, len(d.h))
	for id := range d.h {
		ids = append(ids, id)
	}
	return ids
}

// executeHandlerByID executes a registered cron handler.
func (d *Dispatcher) executeHandlerByID(ctx context.Context, id string) error {
	d.m.RLock()
	h := d.h[id]
	d.m.RUnlock()
	if h == nil {
		return errors.Reason("no cron handler with ID %q is registered", id).Err()
	}
	// TODO(vadimsh): Record performance metrics.
	return h(ctx)
}
