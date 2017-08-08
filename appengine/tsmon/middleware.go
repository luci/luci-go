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

package tsmon

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/appengine"

	"go.chromium.org/gae/service/info"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/iotools"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/monitor"
	"go.chromium.org/luci/common/tsmon/store"
	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"
)

// State holds the configuration of the tsmon library for GAE.
//
// Define it as a global variable and inject it in the request contexts using
// State.Middleware().
//
// It will initialize itself from the tsmon state in the passed context on
// a first use, mutating it along the way. Assumes caller is consistently using
// contexts configured with exact same tsmon state (in a vast majority of cases
// it would be global tsmon state that corresponds to context.Background, but
// unit tests may provide its own state).
//
// Will panic if it detects that caller has changed tsmon state in the context
// between the requests.
type State struct {
	lock sync.RWMutex

	state        *tsmon.State
	lastSettings tsmonSettings

	flushingNow bool
	lastFlushed time.Time

	// testingMonitor is mocked monitor used in unit tests.
	testingMonitor monitor.Monitor
	// testingSettings if not nil are used in unit tests.
	testingSettings *tsmonSettings
}

// responseWriter wraps a given http.ResponseWriter, records its
// status code and response size.
type responseWriter struct {
	http.ResponseWriter
	writer iotools.CountingWriter
	status int
}

func (rw *responseWriter) Write(buf []byte) (int, error) { return rw.writer.Write(buf) }

func (rw *responseWriter) Size() int64 { return rw.writer.Count }

func (rw *responseWriter) Status() int { return rw.status }

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func newResponseWriter(rw http.ResponseWriter) *responseWriter {
	return &responseWriter{
		ResponseWriter: rw,
		writer:         iotools.CountingWriter{Writer: rw},
		status:         http.StatusOK,
	}
}

// Middleware is a middleware that must be inserted into the middleware
// chain to enable tsmon metrics to be send on AppEngine.
func (s *State) Middleware(c *router.Context, next router.Handler) {
	state, settings := s.checkSettings(c.Context)
	if settings.Enabled {
		started := clock.Now(c.Context)
		req := c.Request
		userAgent, ok := req.Header["User-Agent"]
		if !ok || len(userAgent) == 0 {
			userAgent = []string{"Unknown"}
		}
		ctx := c.Context
		contentLength := c.Request.ContentLength
		nrw := newResponseWriter(c.Writer)
		c.Writer = nrw
		defer func() {
			dur := clock.Now(ctx).Sub(started)
			metric.UpdateServerMetrics(ctx, "/", nrw.Status(), dur,
				contentLength, nrw.Size(), userAgent[0])
		}()
		next(c)
		s.flushIfNeeded(ctx, req, state, settings)
	} else {
		next(c)
	}
}

// checkSettings fetches tsmon settings and initializes, reinitializes or
// deinitializes tsmon, as needed.
//
// Returns current tsmon state and settings. Panics if the context is using
// unexpected tsmon state.
func (s *State) checkSettings(c context.Context) (*tsmon.State, *tsmonSettings) {
	state := tsmon.GetState(c)

	var settings tsmonSettings
	if s.testingSettings != nil {
		settings = *s.testingSettings
	} else {
		settings = fetchCachedSettings(c)
	}

	// Read the values used when handling previous request. In most cases they
	// are identical to current ones and we can skip grabbing a heavier write
	// lock.
	s.lock.RLock()
	if s.state == state && s.lastSettings == settings {
		s.lock.RUnlock()
		return state, &settings
	}
	s.lock.RUnlock()

	// 'settings' or 'state' has changed. Reinitialize tsmon as needed under
	// the write lock.
	s.lock.Lock()
	defer s.lock.Unlock()

	// First call to 'checkSettings' ever?
	if s.state == nil {
		s.state = state
		s.state.M = monitor.NewNilMonitor() // doFlush uses its own monitor
		s.state.InvokeGlobalCallbacksOnFlush = false
	} else if state != s.state {
		panic("tsmon state in the context was unexpectedly changed between requests")
	}

	switch {
	case !bool(s.lastSettings.Enabled) && bool(settings.Enabled):
		s.enableTsMon(c)
	case bool(s.lastSettings.Enabled) && !bool(settings.Enabled):
		s.disableTsMon(c)
	}
	s.lastSettings = settings

	return state, &settings
}

// enableTsMon puts in-memory metrics store in the context's tsmon state.
//
// Called with 's.lock' locked.
func (s *State) enableTsMon(c context.Context) {
	s.state.SetStore(store.NewInMemory(&target.Task{
		DataCenter:  targetDataCenter,
		ServiceName: info.AppID(c),
		JobName:     info.ModuleName(c),
		HostName:    strings.SplitN(info.VersionID(c), ".", 2)[0],
		TaskNum:     -1,
	}))

	// Request the flush to be executed ASAP, so it registers (or updates)
	// 'Instance' entity in the datastore.
	s.lastFlushed = time.Time{}
}

// disableTsMon puts nil metrics store in the context's tsmon state.
//
// Called with 's.lock' locked.
func (s *State) disableTsMon(c context.Context) {
	s.state.SetStore(store.NewNilStore())
}

// withGAEContext is replaced in unit tests.
var withGAEContext = appengine.WithContext

// flushIfNeeded periodically flushes the accumulated metrics.
//
// It skips the flush if some other goroutine is already flushing. Logs errors.
func (s *State) flushIfNeeded(c context.Context, req *http.Request, state *tsmon.State, settings *tsmonSettings) {
	now := clock.Now(c)
	flushTime := now.Add(-time.Duration(settings.FlushIntervalSec) * time.Second)

	// Most of the time the flush is not needed and we can get away with
	// lightweight RLock.
	s.lock.RLock()
	skip := s.flushingNow || s.lastFlushed.After(flushTime)
	s.lock.RUnlock()
	if skip {
		return
	}

	// Need to flush. Update flushingNow. Redo the check under write lock.
	s.lock.Lock()
	skip = s.flushingNow || s.lastFlushed.After(flushTime)
	if !skip {
		s.flushingNow = true
	}
	s.lock.Unlock()
	if skip {
		return
	}

	// Report per-process statistic, like memory stats.
	collectProcessMetrics(c, settings)

	// Unset 'flushingNow' no matter what (even on panics). Update 'lastFlushed'
	// only on successful flush. Unsuccessful flush thus will be retried ASAP.
	success := false
	defer func() {
		s.lock.Lock()
		s.flushingNow = false
		if success {
			s.lastFlushed = now
		}
		s.lock.Unlock()
	}()

	// The flush must be fast. Limit it with by some timeout. It also needs real
	// GAE context for gRPC calls in PubSub guts, so slap it on top of luci-go
	// context.
	c, cancel := clock.WithTimeout(c, flushTimeout)
	defer cancel()
	c = withGAEContext(c, req) // TODO(vadimsh): not needed with ProdX
	if err := s.updateInstanceEntityAndFlush(c, state, settings); err != nil {
		logging.Errorf(c, "Failed to flush tsmon metrics: %s", err)
	} else {
		success = true
	}
}

// updateInstanceEntityAndFlush waits for instance to get assigned a task number and
// flushes the metrics.
func (s *State) updateInstanceEntityAndFlush(c context.Context, state *tsmon.State, settings *tsmonSettings) error {
	c = info.MustNamespace(c, instanceNamespace)

	defTarget := state.S.DefaultTarget()
	task, ok := defTarget.(*target.Task)
	if !ok {
		return fmt.Errorf("default tsmon target is not a Task (%T): %v", defTarget, defTarget)
	}

	now := clock.Now(c)

	// Grab TaskNum assigned to the currently running instance. It is updated by
	// a dedicated cron job, see housekeepingHandler in handler.go.
	entity, err := getOrCreateInstanceEntity(c)
	if err != nil {
		return fmt.Errorf("failed to get instance entity - %s", err)
	}

	// Don't do the flush if we still haven't get a task number.
	if entity.TaskNum < 0 {
		if task.TaskNum >= 0 {
			// We used to have a task number but we don't any more (we were inactive
			// for too long), so clear our state.
			logging.Warningf(c, "Instance %s got purged from Datastore, but is still alive. "+
				"Clearing cumulative metrics", info.InstanceID(c))
			state.ResetCumulativeMetrics(c)
		}
		task.TaskNum = -1
		state.S.SetDefaultTarget(task)

		// Complain if we haven't been given a task number yet. This is fine for
		// recently started instances. Task numbers are populated by the
		// housekeeping cron that MUST be configured for tsmon to work.
		logging.Warningf(c, "Skipping the flush: instance %s has no task_num.", info.InstanceID(c))
		if now.Sub(entity.LastUpdated) > instanceExpectedToHaveTaskNum {
			logging.Errorf(c, "Is /internal/cron/ts_mon/housekeeping running? "+
				"The instance %s is %s old and has no task_num yet. This is abnormal.",
				info.InstanceID(c), now.Sub(entity.LastUpdated))
		}

		// Non-assigned TaskNum is expected situation, pretend the flush succeeded,
		// so that next try happens after regular flush period, not right away.
		return nil
	}

	task.TaskNum = int32(entity.TaskNum)
	state.S.SetDefaultTarget(task)

	// Refresh 'entity.LastUpdated'. Ignore errors here since the flush is
	// happening already even this operation fails.
	putDone := make(chan struct{})
	go func() {
		defer close(putDone)
		if err := refreshLastUpdatedTime(c, now); err != nil {
			logging.Errorf(c, "Failed to update instance entity: %s", err)
		}
	}()

	ret := s.doFlush(c, state, settings)

	<-putDone
	return ret
}

// doFlush actually sends the metrics to the monitor.
func (s *State) doFlush(c context.Context, state *tsmon.State, settings *tsmonSettings) error {
	var mon monitor.Monitor
	var err error

	if s.testingMonitor != nil {
		mon = s.testingMonitor
	} else if info.IsDevAppServer(c) || settings.ProdXAccount == "" {
		mon = monitor.NewDebugMonitor("")
	} else {
		logging.Infof(c, "Sending metrics to ProdX using %s", settings.ProdXAccount)
		transport, err := auth.GetRPCTransport(
			c,
			auth.AsActor,
			auth.WithServiceAccount(settings.ProdXAccount),
			auth.WithScopes(monitor.ProdxmonScopes...))
		if err != nil {
			return err
		}
		endpoint, err := url.Parse(prodXEndpoint)
		if err != nil {
			return err
		}
		mon, err = monitor.NewHTTPMonitor(c, &http.Client{Transport: transport}, endpoint)
		if err != nil {
			return err
		}
	}

	defer mon.Close()
	if err = state.Flush(c, mon); err != nil {
		return err
	}

	return nil
}
