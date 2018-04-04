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

package tsmon

import (
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/monitor"
	"go.chromium.org/luci/common/tsmon/runtimestats"
	"go.chromium.org/luci/common/tsmon/store"
	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/common/tsmon/versions"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"
)

// State holds the state and configuration of the tsmon library.
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
	// Target is lazily called to initialize default metrics target.
	//
	// The target identifies the collection of homogeneous processes that together
	// implement the service. Each individual process in the collection is
	// additionally identified by a task number, later dynamically assigned via
	// TaskNumAllocator based on unique InstanceID.
	Target func(c context.Context) target.Task

	// InstanceID returns a unique (within the scope of the service) identifier of
	// this particular process.
	//
	// It will be used to assign a free task number via TaskNumAllocator.
	InstanceID func(c context.Context) string

	// TaskNumAllocator knows how to dynamically map task ID to a task number.
	TaskNumAllocator TaskNumAllocator

	// IsDevMode should be set to true when running locally.
	IsDevMode bool

	lock sync.RWMutex

	state        *tsmon.State
	lastSettings tsmonSettings

	instanceID  string    // cached result of InstanceID() call
	flushingNow bool      // true if some goroutine is flushing right now
	nextFlush   time.Time // next time we should do the flush
	lastFlush   time.Time // last successful flush

	testingMonitor  monitor.Monitor // mocked in unit tests
	testingSettings *tsmonSettings  // mocked in unit tests
}

const (
	// noFlushErrorThreshold defines when we start to complain in error log that
	// the last successful flush (if ever) was too long ago.
	noFlushErrorThreshold = 5 * time.Minute

	// flushTimeout defines a deadline for the flush operation.
	flushTimeout = 5 * time.Second
)

// Middleware is a middleware that collects request metrics and triggers metric
// flushes.
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
			metric.UpdateServerMetrics(ctx, c.HandlerPath, nrw.Status(), dur,
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
	t := s.Target(c)
	t.TaskNum = -1 // will be assigned later via TaskNumAllocator

	s.state.SetStore(store.NewInMemory(&t))

	// Request the flush to be executed ASAP, so it registers a claim for a task
	// number via NotifyTaskIsAlive. Also reset 'lastFlush', so that we don't get
	// invalid logging that the last flush was long time ago.
	s.nextFlush = clock.Now(c)
	s.lastFlush = s.nextFlush
}

// disableTsMon puts nil metrics store in the context's tsmon state.
//
// Called with 's.lock' locked.
func (s *State) disableTsMon(c context.Context) {
	s.state.SetStore(store.NewNilStore())
}

// flushIfNeeded periodically flushes the accumulated metrics.
//
// It skips the flush if some other goroutine is already flushing. Logs errors.
func (s *State) flushIfNeeded(c context.Context, req *http.Request, state *tsmon.State, settings *tsmonSettings) {
	now := clock.Now(c)

	// Most of the time the flush is not needed and we can get away with
	// lightweight RLock.
	s.lock.RLock()
	skip := s.flushingNow || now.Before(s.nextFlush)
	s.lock.RUnlock()
	if skip {
		return
	}

	// Need to flush. Update flushingNow. Redo the check under write lock, as well
	// as do a bunch of other calls while we hold the lock. Will be useful later.
	s.lock.Lock()
	if s.instanceID == "" {
		s.instanceID = s.InstanceID(c)
	}
	instanceID := s.instanceID
	lastFlush := s.lastFlush
	skip = s.flushingNow || now.Before(s.nextFlush)
	if !skip {
		s.flushingNow = true
	}
	s.lock.Unlock()
	if skip {
		return
	}

	// The flush must be fast. Limit it by some timeout.
	c, cancel := clock.WithTimeout(c, flushTimeout)
	defer cancel()

	// Report per-process statistic.
	versions.Report(c)
	if settings.ReportRuntimeStats {
		runtimestats.Report(c)
	}

	// Unset 'flushingNow' no matter what (even on panics). Update 'nextFlush'
	// only on successful flush or when we are still waiting for the task number.
	// Unsuccessful flush thus will be retried ASAP.
	var err error
	defer func() {
		s.lock.Lock()
		defer s.lock.Unlock()
		s.flushingNow = false
		if err == nil || err == ErrNoTaskNumber {
			s.nextFlush = now.Add(time.Duration(settings.FlushIntervalSec) * time.Second)
			if err == nil {
				s.lastFlush = now
			}
		}
	}()

	err = s.ensureTaskNumAndFlush(c, instanceID, state, settings)
	if err != nil {
		if err == ErrNoTaskNumber {
			logging.Warningf(c, "Skipping the tsmon flush: no task number assigned yet")
		} else {
			logging.WithError(err).Errorf(c, "Failed to flush tsmon metrics")
		}
		if sinceLastFlush := now.Sub(lastFlush); sinceLastFlush > noFlushErrorThreshold {
			logging.Errorf(c, "No successful tsmon flush for %s. Is /internal/cron/ts_mon/housekeeping running?", sinceLastFlush)
		}
	}
}

// ensureTaskNumAndFlush gets a task number assigned to the process and flushes
// the metrics.
//
// Returns ErrNoTaskNumber if the task wasn't assigned a task number yet.
func (s *State) ensureTaskNumAndFlush(c context.Context, instanceID string, state *tsmon.State, settings *tsmonSettings) error {
	var task target.Task
	defTarget := state.S.DefaultTarget()
	if t, ok := defTarget.(*target.Task); ok {
		task = *t
	} else {
		return fmt.Errorf("default tsmon target is not a Task (%T): %v", defTarget, defTarget)
	}

	// Notify the task number allocator that we are still alive and grab the
	// TaskNum assigned to us.
	assignedTaskNum, err := s.TaskNumAllocator.NotifyTaskIsAlive(c, &task, instanceID)
	if err != nil && err != ErrNoTaskNumber {
		return fmt.Errorf("failed to get task number assigned for %q - %s", instanceID, err)
	}

	// Don't do the flush if we still haven't got a task number.
	if err == ErrNoTaskNumber {
		if task.TaskNum >= 0 {
			logging.Warningf(c, "The task was inactive for too long and lost its task number, clearing cumulative metrics")
			state.ResetCumulativeMetrics(c)
		}
		task.TaskNum = -1
		state.S.SetDefaultTarget(&task)
		return ErrNoTaskNumber
	}

	task.TaskNum = int32(assignedTaskNum)
	state.S.SetDefaultTarget(&task)
	return s.doFlush(c, state, settings)
}

// doFlush actually sends the metrics to the monitor.
func (s *State) doFlush(c context.Context, state *tsmon.State, settings *tsmonSettings) error {
	var mon monitor.Monitor
	var err error

	if s.testingMonitor != nil {
		mon = s.testingMonitor
	} else if s.IsDevMode || settings.ProdXAccount == "" {
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
