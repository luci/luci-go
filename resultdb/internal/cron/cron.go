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
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/runtime/paniccatcher"
)

// pollReplicas periodically checks the value returned by replicasFunc, and
// when it changes, it calls the given context cancellation function and sets
// the pointed boolean to true.
func pollReplicas(ctx context.Context, replicasFunc func(context.Context) int, cancel context.CancelFunc, resume *bool) {
	replicas := replicasFunc(ctx)
	for {
		time.Sleep(30 * time.Second)
		select {
		case <-ctx.Done():
			return
		default:
			if replicasFunc(ctx) != replicas {
				*resume = true
				cancel()
				return
			}
		}
	}
}

// DynamicGroup wraps Group to periodically check the right number of replicas.
func DynamicGroup(
	ctx context.Context,
	replicasFunc func(context.Context) int,
	minInterval time.Duration,
	payloadFunc func(ctx context.Context, replica int) error) {

	resume := false
	derivedCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	for {
		// if the number of replicas changes, this will cancel the context and
		// set the continuation flag.
		go pollReplicas(derivedCtx, replicasFunc, cancel, &resume)

		Group(derivedCtx, replicasFunc(derivedCtx), minInterval, payloadFunc)

		if resume {
			// pollReplicas detected a change in the number of replicas.
			// Reset the context and flag and restart the pool with the new
			// number of replicas.
			derivedCtx, cancel = context.WithCancel(ctx)
			resume = false
			continue
		}
		break
	}
}

// Group runs multiple cron jobs concurrently. See also Run function.
func Group(ctx context.Context, replicas int, minInterval time.Duration, f func(ctx context.Context, replica int) error) {
	var wg sync.WaitGroup
	for i := 0; i < replicas; i++ {
		i := i
		ctx := logging.SetField(ctx, "cron_replica", i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			Run(ctx, minInterval, func(ctx context.Context) error {
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
	defer logging.Warningf(ctx, "Exiting cron")

	// call calls f with a timeout and catches a panic.
	call := func(ctx context.Context) error {
		defer paniccatcher.Catch(func(p *paniccatcher.Panic) {
			logging.Errorf(ctx, "Caught panic: %s\n%s", p.Reason, p.Stack)
		})
		return f(ctx)
	}

	var iterationCounter int
	logLimiter := rate.NewLimiter(rate.Every(5*time.Minute), 1)
	for {
		iterationCounter++
		if logLimiter.Allow() {
			logging.Debugf(ctx, "%d iterations have run since start-up", iterationCounter)
		}

		start := clock.Now(ctx)
		if err := call(ctx); err != nil {
			logging.Errorf(ctx, "Iteration failed: %s", err)
		}

		// Ensure minInterval between iterations.
		if sleep := minInterval - clock.Since(ctx, start); sleep > 0 {
			// Add jitter: +-10% of sleep time to desynchronize cron jobs.
			sleep = sleep - sleep/10 + time.Duration(mathrand.Intn(ctx, int(sleep/5)))
			select {
			case <-time.After(sleep):
			case <-ctx.Done():
				return
			}
		}
	}
}
