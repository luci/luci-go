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

// Package cron can runs functions periodically.
package cron

import (
	"context"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/runtime/paniccatcher"
)

// ThrottledLogger emits a log item at most every MinLogInterval.
//
// Its zero-value emits a debug-level log every time Log() is called.
type ThrottledLogger struct {
	MinLogInterval time.Duration
	Level          logging.Level
	lastLog        time.Time
	mu             sync.Mutex
}

// Log emits a log item at most once every MinLogInterval.
func (tl *ThrottledLogger) Log(ctx context.Context, format string, args ...interface{}) {
	now := clock.Now(ctx)
	tl.mu.Lock()
	if now.Sub(tl.lastLog) > tl.MinLogInterval {
		logging.Logf(ctx, tl.Level, format, args...)
		tl.lastLog = now
	}
	tl.mu.Unlock()
}

// Group runs multiple cron jobs concurrently. See also Run function.
func Group(ctx context.Context, replicas int, minInterval time.Duration, f func(ctx context.Context, replica int) error) {
	var wg sync.WaitGroup
	l := rate.NewLimiter(rate.Every(minInterval), 1)
	for i := 0; i < replicas; i++ {
		i := i
		ctx := logging.SetField(ctx, "cron_replica", i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			run(ctx, l, func(ctx context.Context) error {
				return f(ctx, i)
			})
		}()
	}
	wg.Wait()
}

// Run runs f repeatedly, until the context is cancelled.
//
// Ensures f is not called too often (minInterval).
func Run(ctx context.Context, minInterval time.Duration, f func(context.Context) error) {
	run(ctx, rate.NewLimiter(rate.Every(minInterval), 1), f)
}

func run(ctx context.Context, l *rate.Limiter, f func(context.Context) error) {
	defer logging.Warningf(ctx, "Exiting cron")

	// call calls f with a timeout and catches a panic.
	call := func(ctx context.Context) error {
		defer paniccatcher.Catch(func(p *paniccatcher.Panic) {
			logging.Errorf(ctx, "Caught panic: %s\n%s", p.Reason, p.Stack)
		})
		if err := l.Wait(ctx); err != nil {
			return errors.Annotate(err, "failed waiting limiter").Err()
		}
		return f(ctx)
	}

	var iterationCounter int
	tl := ThrottledLogger{MinLogInterval: 5 * time.Minute}
	for ctx.Err() == nil {
		iterationCounter++
		tl.Log(ctx, "%d iterations have run since start-up", iterationCounter)
		if err := call(ctx); err != nil {
			logging.Errorf(ctx, "Iteration failed: %s", err)
		}
	}
}
