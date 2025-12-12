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
	"regexp"
	"sort"
	"strings"
	"sync"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"

	"go.chromium.org/luci/server/internal"
	"go.chromium.org/luci/server/router"
)

var (
	callsCounter = metric.NewCounter(
		"cron/server/calls",
		"Count of handled cron job invocations",
		nil,
		field.String("id"),     // cron handler ID
		field.String("result"), // OK | transient | fatal | panic | no_handler | auth
	)

	callsDurationMS = metric.NewCumulativeDistribution(
		"cron/server/duration",
		"Duration of handling of recognized handlers",
		&types.MetricMetadata{Units: types.Milliseconds},
		distribution.DefaultBucketer,
		field.String("id"),     // cron handler ID
		field.String("result"), // OK | transient | fatal | panic
	)
)

// Handler is called to handle a cron job invocation.
//
// Transient errors are transformed into HTTP 500 replies to Cloud Scheduler,
// which may trigger a retry based on the job's retry configuration. Returning a
// non-transient error results in a error-level logging message and HTTP 202
// reply, which does not trigger a retry.
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

	// DisableAuth can be used to disable authentication on HTTP endpoints.
	//
	// This is useful when running in development mode on localhost or in tests.
	DisableAuth bool

	m sync.RWMutex
	h map[string]Handler
}

// handlerIDRe is used to validate handler IDs.
var handlerIDRe = regexp.MustCompile(`^[a-zA-Z0-9_\-.]{1,100}$`)

// RegisterHandler registers a callback called to handle a cron job invocation.
//
// The handler can be invoked via GET requests to "<serving-prefix>/<id>",
// (usually "/internal/cron/<id>"). This URL path should be used when
// configuring Cloud Scheduler jobs or in cron.yaml when running on Appengine.
//
// The ID must match `[a-zA-Z0-9_\-.]{1,100}`. Panics otherwise. Panics if a
// handler with such ID is already registered.
func (d *Dispatcher) RegisterHandler(id string, h Handler) {
	if !handlerIDRe.MatchString(id) {
		panic(fmt.Sprintf("bad cron handler ID %q", id))
	}
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

	route := strings.TrimRight(prefix, "/") + "/*handler"
	handlerID := func(c *router.Context) string {
		return strings.TrimPrefix(c.Params.ByName("handler"), "/")
	}

	var mw router.MiddlewareChain
	if !d.DisableAuth {
		header := ""
		if d.GAE {
			header = "X-Appengine-Cron"
		}
		mw = internal.CloudAuthMiddleware(d.AuthorizedCallers, header,
			func(c *router.Context) {
				callsCounter.Add(c.Request.Context(), 1, handlerID(c), "auth")
			},
		)
	}

	r.GET(route, mw, func(c *router.Context) {
		id := handlerID(c)
		if err := d.executeHandlerByID(c.Request.Context(), id); err != nil {
			if transient.Tag.In(err) {
				logging.Warningf(c.Request.Context(), "transient error in cron handler %q: %w", id, err)
				http.Error(c.Writer, err.Error(), 500)
			} else {
				errors.Log(c.Request.Context(), errors.Fmt("fatal error in cron handler %q: %w", id, err))
				http.Error(c.Writer, err.Error(), 202)
			}
		} else {
			c.Writer.Write([]byte("OK"))
		}
	})
}

// handlerIDs returns a sorted list of registered handler IDs.
func (d *Dispatcher) handlerIDs() []string {
	d.m.RLock()
	defer d.m.RUnlock()
	ids := make([]string, 0, len(d.h))
	for id := range d.h {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// executeHandlerByID executes a registered cron handler.
func (d *Dispatcher) executeHandlerByID(ctx context.Context, id string) error {
	d.m.RLock()
	h := d.h[id]
	d.m.RUnlock()
	if h == nil {
		callsCounter.Add(ctx, 1, id, "no_handler")
		return errors.Fmt("no cron handler with ID %q is registered", id)
	}

	start := clock.Now(ctx)
	result := "panic"
	defer func() {
		callsCounter.Add(ctx, 1, id, result)
		callsDurationMS.Add(ctx, float64(clock.Since(ctx, start).Milliseconds()), id, result)
	}()

	err := h(ctx)
	switch {
	case err == nil:
		result = "OK"
	case transient.Tag.In(err):
		result = "transient"
	default:
		result = "fatal"
	}
	return err
}
