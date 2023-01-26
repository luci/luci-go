// Copyright 2019 The LUCI Authors.
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

// Package host implements the 'Host Application' portion of the luciexe
// protocol.
//
// It manages a local Logdog Butler service, and also runs all LUCI Auth related
// daemons. It intercepts and interprets build.proto streams within the Butler
// context, merging them as necessary.
package host

import (
	"context"
	"strings"
	"time"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/lucictx"
)

var maxLogFlushWaitTime = 30 * time.Second

// Run executes `cb` in a "luciexe" host environment.
//
// The merged Build objects collected from the host environment (i.e. generated
// within `cb`) will be pushed to the returned "<-chan *bbpb.Build" as `cb`
// executes.
//
// Error during starting up the host environment will be directly returned.
// But `cb` does not return anything to avoid messy semantics because it's run
// in a goroutine; If `cb` could error out, it's recommended to make your own
// `chan error` and have `cb` push to that.
//
// The context should be used for cancellation of the callback function; It's up
// to the `cb` implementation to respect the cancelled context.
//
// When the callback function completes, Run closes the returned channel.
//
// Blocking the returned channel may block the execution of `cb`.
//
// NOTE: This modifies the environment (i.e. with os.Setenv) while `cb` is
// running. Be careful when using Run concurrently with other code. You MUST
// completely drain the returned channel in order to be guaranteed that all
// side-effects of Run have been unwound.
func Run(ctx context.Context, options *Options, cb func(context.Context, Options, <-chan lucictx.DeadlineEvent, func())) (<-chan *bbpb.Build, error) {
	var opts Options
	if options != nil {
		opts = *options
	}
	if err := opts.initialize(); err != nil {
		return nil, err
	}
	logging.Infof(ctx, "starting luciexe host env with: %+v", opts)

	// cleanup will accumulate all of the cleanup functions as we set up the
	// environment. If an error occurs before we can start the user code (`cb`),
	// the defer below will run them all. Otherwise they'll be transferred to the
	// goroutine.
	var cleanup cleanupSlice
	defer cleanup.run(ctx)

	cleanupComplete := make(chan struct{})
	cleanup.add("cleanupComplete", func() error {
		close(cleanupComplete)
		return nil
	})

	// First, capture the entire env to restore it later.
	cleanup.add("restoreEnv", restoreEnv())

	logging.Infof(ctx, "starting auth services")
	if err := cleanup.concat(startAuthServices(ctx, &opts)); err != nil {
		return nil, err
	}

	logging.Infof(ctx, "starting butler")
	butler, err := startButler(ctx, &opts)
	if err != nil {
		return nil, err
	}
	cleanup.add("butler", func() error {
		butler.Activate()
		return butler.Wait()
	})

	logging.Infof(ctx, "starting build.proto merging agent")
	agent, err := spyOn(ctx, butler, opts.BaseBuild)
	if err != nil {
		return nil, err
	}
	cleanup.add("buildmerge spy", func() error {
		agent.Close()
		logging.Infof(ctx, "waiting for buildmerge spy to finish")
		<-agent.DrainC
		return nil
	})

	buildCh := make(chan *bbpb.Build)
	go func() {
		defer close(buildCh)
		for build := range agent.MergedBuildC {
			buildCh <- build
		}
		<-cleanupComplete
	}()

	// Transfer ownership of cleanups to goroutine
	userCleanup := cleanup
	userCleanup.add("flush u/", func() error {
		wt := calcLogFlushWaitTime(ctx)
		cctx, cancel := clock.WithTimeout(ctx, wt)
		defer cancel()
		logging.Infof(ctx, "waiting up to %s for user logs to flush", wt)
		leftovers := butler.DrainNamespace(cctx, agent.UserNamespace)
		if len(leftovers) > 0 {
			builder := strings.Builder{}
			for _, leftover := range leftovers {
				builder.WriteString("\n  ")
				builder.WriteString(string(leftover))
			}
			logging.Errorf(
				ctx, "failed to flush the following logs:\n  %s", builder.String())
		}
		return nil
	})
	userCleanup.add("butler.Activate", func() error {
		butler.Activate()
		return nil
	})
	cleanup = nil

	// Buildbucket assigns some grace period to the surrounding task which is
	// more than what the user requested in `input.Build.GracePeriod`. We
	// reserve the difference here so the user task only gets what they asked
	// for.
	deadline := lucictx.GetDeadline(ctx)
	toReserve := deadline.GracePeriodDuration() - opts.BaseBuild.GracePeriod.AsDuration()
	logging.Infof(
		ctx, "Reserving %s out of %s of grace_period from LUCI_CONTEXT.",
		toReserve, lucictx.GetDeadline(ctx).GracePeriodDuration())
	dctx, shutdown := lucictx.TrackSoftDeadline(ctx, toReserve)

	go func() {
		defer userCleanup.run(ctx)
		logging.Infof(ctx, "invoking host environment callback")

		cb(dctx, opts, lucictx.SoftDeadlineDone(dctx), shutdown)
	}()

	return buildCh, nil
}

// If ctx has the deadline set, waitTime is min(half of the remaining time
// towards deadline, `maxLogFlushWaitTime`). Otherwise, waitTime is the same
// as `maxLogFlushWaitTime`.
func calcLogFlushWaitTime(ctx context.Context) time.Duration {
	if deadline, ok := ctx.Deadline(); ok {
		switch waitTime := deadline.Sub(clock.Now(ctx)) / 2; {
		case waitTime < 0:
			return 0
		case waitTime > maxLogFlushWaitTime:
			return maxLogFlushWaitTime
		default:
			return waitTime
		}
	}
	return maxLogFlushWaitTime
}
