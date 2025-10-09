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
	"context"
	"sync"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/iotools"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/monitor"
	"go.chromium.org/luci/common/tsmon/runtimestats"
	"go.chromium.org/luci/common/tsmon/store"
	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/common/tsmon/versions"

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
	// additionally identified by a task number, later (optionally) dynamically
	// assigned via TaskNumAllocator based on unique InstanceID.
	Target func(ctx context.Context) target.Task

	// InstanceID returns a unique (within the scope of the service) identifier of
	// this particular process.
	//
	// It will be used to assign a free task number via TaskNumAllocator.
	//
	// If nil, instance ID will be set to "". Useful when TaskNumAllocator is nil
	// too (then instance ID is not important).
	InstanceID func(ctx context.Context) string

	// TaskNumAllocator knows how to dynamically map instance ID to a task number.
	//
	// If nil, 0 will be used as task number regardless of instance ID.
	TaskNumAllocator TaskNumAllocator

	// CustomMonitor, if not nil, is used to flush accumulated metrics instead of
	// the default monitor (which is constructed on the fly based on current
	// settings).
	//
	// Useful to override the default monitor in tests or when running locally in
	// the debug mode.
	CustomMonitor monitor.Monitor

	// FlushInMiddleware is true to make Middleware(...) periodically
	// synchronously send metrics to the backend after handling a request.
	//
	// This is useful on Standard GAE that doesn't support asynchronous flushes
	// outside of a context of some request.
	//
	// If false, the user of the library *must* launch FlushPeriodically() in
	// a background goroutine. Otherwise metrics won't be sent.
	FlushInMiddleware bool

	// Settings, if non nil, are static preset settings to use.
	//
	// If nil, settings will be loaded dynamically through 'settings' module.
	Settings *Settings

	lock sync.RWMutex

	state        *tsmon.State
	lastSettings Settings

	instanceID  string        // cached result of InstanceID() call
	flushingNow bool          // true if some goroutine is flushing right now
	nextFlush   time.Time     // next time we should do the flush
	lastFlush   time.Time     // last successful flush
	flushRetry  time.Duration // flush retry delay
}

const (
	// noFlushErrorThreshold defines when we start to complain in error log that
	// the last successful flush (if ever) was too long ago.
	noFlushErrorThreshold = 5 * time.Minute

	// flushMaxRetry defines the maximum delay between flush retries.
	flushMaxRetry = 10 * time.Minute

	// flushInitialRetry defines the initial retry delay. This
	// is doubled every retry, up to flushMaxRetry.
	flushInitialRetry = 5 * time.Second
)

// Activate updates the tsmon state in the context to use this config.
//
// Not needed if metrics are updated only from inside the middleware.
func (s *State) Activate(ctx context.Context) {
	s.checkSettings(ctx)
}

// Middleware is a middleware that collects request metrics and triggers metric
// flushes.
func (s *State) Middleware(c *router.Context, next router.Handler) {
	state, settings := s.checkSettings(c.Request.Context())
	if settings.Enabled {
		started := clock.Now(c.Request.Context())
		req := c.Request
		userAgent, ok := req.Header["User-Agent"]
		if !ok || len(userAgent) == 0 {
			userAgent = []string{"Unknown"}
		}
		ctx := c.Request.Context()
		contentLength := c.Request.ContentLength
		nrw := iotools.NewResponseWriter(c.Writer)
		c.Writer = nrw
		defer func() {
			dur := clock.Now(ctx).Sub(started)
			metric.UpdateServerMetrics(ctx, c.HandlerPath, nrw.Status(), dur,
				contentLength, nrw.ResponseSize(), userAgent[0])
		}()
		next(c)
		if s.FlushInMiddleware {
			s.flushIfNeededImpl(ctx, state, settings)
		}
	} else {
		next(c)
	}
}

// FlushPeriodically runs a loop that periodically flushes metrics.
//
// Blocks until the context is canceled. Handles (and logs) errors internally.
func (s *State) FlushPeriodically(ctx context.Context) {
	for {
		var nextFlush time.Time
		state, settings := s.checkSettings(ctx)
		if settings.Enabled {
			nextFlush = s.flushIfNeededImpl(ctx, state, settings)
		}
		// Don't sleep less than 1 sec to avoid busy looping. It is always OK to
		// flush a sec later. In most cases we'll be sleeping ~= FlushIntervalSec.
		sleep := time.Second
		if dt := nextFlush.Sub(clock.Now(ctx)); dt > sleep {
			sleep = dt
		}
		if r := <-clock.After(ctx, sleep); r.Err != nil {
			return // the context is canceled
		}
	}
}

// checkSettings fetches tsmon settings and initializes, reinitializes or
// deinitializes tsmon, as needed.
//
// Returns current tsmon state and settings. Panics if the context is using
// unexpected tsmon state.
func (s *State) checkSettings(ctx context.Context) (*tsmon.State, *Settings) {
	state := tsmon.GetState(ctx)

	var settings Settings
	if s.Settings != nil {
		settings = *s.Settings
	} else {
		settings = fetchCachedSettings(ctx)
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
		s.state.SetMonitor(monitor.NewNilMonitor())
		s.state.InhibitGlobalCallbacksOnFlush()
		if s.InstanceID != nil {
			s.instanceID = s.InstanceID(ctx)
		}
	} else if state != s.state {
		panic("tsmon state in the context was unexpectedly changed between requests")
	}

	switch {
	case !bool(s.lastSettings.Enabled) && bool(settings.Enabled):
		s.enableTSMon(ctx, settings.ProdXAccount)
	case bool(s.lastSettings.Enabled) && !bool(settings.Enabled):
		s.disableTSMon(ctx)
	}
	s.lastSettings = settings

	return state, &settings
}

// enableTSMon puts in-memory metrics store in the context's tsmon state.
//
// Called with 's.lock' locked.
func (s *State) enableTSMon(ctx context.Context, prodXAccount string) {
	t := s.Target(ctx)
	t.TaskNum = -1 // will be assigned later via TaskNumAllocator

	s.state.SetStore(store.NewInMemory(&t))

	// Request the flush to be executed ASAP, so it registers a claim for a task
	// number via NotifyTaskIsAlive. Also reset 'lastFlush', so that we don't get
	// invalid logging that the last flush was long time ago.
	s.nextFlush = clock.Now(ctx)
	s.lastFlush = s.nextFlush
	// Set initial value for retry delay.
	s.flushRetry = flushInitialRetry

	mon := s.CustomMonitor
	if mon == nil {
		prodx, err := NewProdXMonitor(ctx, 500, prodXAccount)
		if err != nil {
			logging.WithError(err).Errorf(ctx, "failed to dial ProdX; using NilMonitor()")
			mon = monitor.NewNilMonitor()
		} else {
			mon = prodx
		}
	}
	s.state.SetMonitor(mon)
}

// disableTSMon puts nil metrics store in the context's tsmon state.
//
// Called with 's.lock' locked.
func (s *State) disableTSMon(ctx context.Context) {
	s.state.SetStore(store.NewNilStore())
	if mon := s.state.Monitor(); mon != nil {
		if err := mon.Close(); err != nil {
			logging.WithError(err).Errorf(ctx, "failed to close tsmon monitor; ignoring")
		}
	}
}

// flushIfNeededImpl periodically flushes the accumulated metrics.
//
// It skips the flush if some other goroutine is already flushing. Logs errors.
//
// Returns a timestamp when the next flush should occur. It may be in the past
// if the flush is happening concurrently right now in another goroutine.
//
// TODO(vadimsh): Refactor flushIfNeededImpl + FlushPeriodically to be less
// awkward. Historically, flushIfNeededImpl was used only from Middleware and
// FlushPeriodically was slapped on top later.
func (s *State) flushIfNeededImpl(ctx context.Context, state *tsmon.State, settings *Settings) (nextFlush time.Time) {
	// Used to compare 'nextFlush' to 'now'. Needed to make sure we really do
	// the flush after sleeping in FlushPeriodically even if we happened to wake
	// up a few nanoseconds earlier.
	const epsilonT = 100 * time.Millisecond

	now := clock.Now(ctx)

	// Most of the time the flush is not needed and we can get away with
	// lightweight RLock.
	s.lock.RLock()
	nextFlush = s.nextFlush
	skip := s.flushingNow || now.Add(epsilonT).Before(nextFlush)
	s.lock.RUnlock()
	if skip {
		return
	}

	// Need to flush. Update flushingNow. Redo the check under write lock, as well
	// as do a bunch of other calls while we hold the lock. Will be useful later.
	s.lock.Lock()
	lastFlush := s.lastFlush
	nextFlush = s.nextFlush
	skip = s.flushingNow || now.Add(epsilonT).Before(nextFlush)
	if !skip {
		s.flushingNow = true
	}
	s.lock.Unlock()
	if skip {
		return
	}

	// The flush must be fast. Limit it by some timeout.
	timeout := settings.FlushTimeoutSec
	if timeout == 0 {
		timeout = defaultSettings.FlushTimeoutSec
	}
	ctx, cancel := clock.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	// Report per-process statistic.
	versions.Report(ctx)
	if settings.ReportRuntimeStats {
		runtimestats.Report(ctx)
	}

	// Unset 'flushingNow' no matter what (even on panics).
	// If flush has failed, retry with back off to avoid
	// hammering the receiver.
	var err error
	defer func() {
		s.lock.Lock()
		defer s.lock.Unlock()
		s.flushingNow = false
		if err == nil || err == ErrNoTaskNumber {
			// Reset retry delay.
			s.flushRetry = flushInitialRetry
			s.nextFlush = now.Add(time.Duration(settings.FlushIntervalSec) * time.Second)
			if err == nil {
				s.lastFlush = now
			}
		} else {
			// Flush has failed, back off the next flush.
			if s.flushRetry < flushMaxRetry {
				s.flushRetry *= 2
			}
			s.nextFlush = now.Add(s.flushRetry)
		}
		nextFlush = s.nextFlush
	}()

	err = s.ensureTaskNumAndFlush(ctx, state, settings)
	if err != nil {
		if err == ErrNoTaskNumber {
			logging.Warningf(ctx, "Skipping the tsmon flush: no task number assigned yet")
		} else if !errors.Is(err, context.Canceled) {
			logging.WithError(err).Warningf(ctx, "Failed to flush tsmon metrics (tried to act as %q)", settings.ProdXAccount)
		}
		if sinceLastFlush := now.Sub(lastFlush); sinceLastFlush > noFlushErrorThreshold {
			logging.WithError(err).Errorf(ctx, "No successful tsmon flush for %s", sinceLastFlush)
			if s.TaskNumAllocator != nil {
				logging.Errorf(ctx, "Is /internal/cron/ts_mon/housekeeping running?")
			}
		}
	}

	// 'nextFlush' return value is constructed in the defer.
	return
}

// ensureTaskNumAndFlush gets a task number assigned to the process and flushes
// the metrics.
//
// Returns ErrNoTaskNumber if the task wasn't assigned a task number yet.
func (s *State) ensureTaskNumAndFlush(ctx context.Context, state *tsmon.State, settings *Settings) error {
	task := *state.Store().DefaultTarget().(*target.Task)
	if s.TaskNumAllocator != nil {
		// Notify the task number allocator that we are still alive and grab
		// the TaskNum assigned to us.
		switch num, err := s.TaskNumAllocator.NotifyTaskIsAlive(ctx, &task, s.instanceID); {
		case err == nil:
			task.TaskNum = int32(num)
		case err == ErrNoTaskNumber:
			if task.TaskNum >= 0 {
				logging.Warningf(ctx, "The task was inactive for too long and lost its task number, clearing cumulative metrics")
				state.ResetCumulativeMetrics(ctx)

				// set the task num to -1 to stop resetting cumulative metrics,
				// in case ErrNoTaskNumber is repeated.
				task.TaskNum = -1
				state.Store().SetDefaultTarget(&task)
			}
			return err
		default:
			return errors.Fmt("failed to get a new task num for %q: %w", s.instanceID, err)
		}
	} else {
		task.TaskNum = 0
	}

	workers := settings.NumFlushWorkers
	if workers == 0 {
		workers = defaultSettings.NumFlushWorkers
	}

	state.Store().SetDefaultTarget(&task)
	return state.ParallelFlush(ctx, state.Monitor(), workers)
}
