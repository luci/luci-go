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
	"fmt"
	"strings"
	"time"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/cipd/client/cipd/platform"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/lucictx"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
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

	buildCh := make(chan *bbpb.Build)
	go func() {
		// cleanup will accumulate all of the cleanup functions as we set up the
		// environment.
		var cleanup cleanupSlice
		defer func() {
			cleanup.run(ctx)
			close(buildCh)
		}()

		// First, capture the entire env to restore it later.
		cleanup.add("restoreEnv", restoreEnv())

		// agent inputs must be downloaded before other services because it may
		// return build when failed. Otherwise the build will always be "overwritten"
		// by agent when shutdown.
		if opts.DownloadAgentInputs {
			if ok := downloadInputs(ctx, opts.agentInputsDir, opts.CacheDir, opts.BaseBuild, buildCh); !ok {
				return
			}
		}

		if err := cleanup.concat(startServices(ctx, &opts, buildCh)); err != nil {
			opts.BaseBuild.Output = &bbpb.Build_Output{
				Status:          bbpb.Status_INFRA_FAILURE,
				SummaryMarkdown: fmt.Sprintf("Failed to start luciexe host: %s", err.Error()),
			}
			// we can send opts.BaseBuild without cloning - we're quitting at this
			// point so cannot do more writes.
			buildCh <- opts.BaseBuild
			return
		}

		// Buildbucket assigns some grace period to the surrounding task which is
		// more than what the user requested in `input.Build.GracePeriod`. We
		// reserve the difference here so the user task only gets what they asked
		// for.
		deadline := lucictx.GetDeadline(ctx)
		toReserve := deadline.GracePeriodDuration() - opts.BaseBuild.GracePeriod.AsDuration()
		if toReserve < 0 {
			logging.Warningf(
				ctx, "Cannot reserve %s grace period for build: LUCI_CONTEXT only has %s of grace_period available",
				opts.BaseBuild.GracePeriod.AsDuration(), deadline.GracePeriodDuration())
			logging.Warningf(
				ctx, "Reserving entire duration - build will operate with an effective grace_period of zero.")
			toReserve = deadline.GracePeriodDuration()
		} else {
			logging.Infof(
				ctx, "Reserving %s out of %s of grace_period from LUCI_CONTEXT.",
				toReserve, deadline.GracePeriodDuration())
		}
		dctx, shutdown := lucictx.TrackSoftDeadline(ctx, toReserve)

		// Invoking luciexe callback.
		logging.Infof(ctx, "invoking host environment callback")
		cb(dctx, opts, lucictx.SoftDeadlineDone(dctx), shutdown)
	}()
	return buildCh, nil
}

func startServices(ctx context.Context, opts *Options, buildCh chan<- *bbpb.Build) (cleanupSlice, error) {
	var myCleanups cleanupSlice
	defer myCleanups.run(ctx)

	logging.Infof(ctx, "starting auth services")
	if err := myCleanups.concat(startAuthServices(ctx, opts)); err != nil {
		return nil, err
	}

	logging.Infof(ctx, "starting butler")
	butler, err := startButler(ctx, opts)
	if err != nil {
		return nil, err
	}
	myCleanups.add("butler", func() error {
		butler.Activate()
		return butler.Wait()
	})

	logging.Infof(ctx, "starting build.proto merging agent")
	agent, err := spyOn(ctx, butler, opts.BaseBuild)
	if err != nil {
		return nil, err
	}

	// forwardComplete indicates all builds has been forwarded through buildCh
	// and now buildCh is safe to be closed. Because buildCh will be closed
	// immediately after cleanup finished, block the cleanup function to wait
	// until forwarding is completed.
	forwardComplete := make(chan struct{})
	myCleanups.add("buildmerge spy", func() error {
		agent.Close()
		logging.Infof(ctx, "waiting for buildmerge spy to finish")
		<-agent.DrainC
		<-forwardComplete
		return nil
	})

	go func() {
		defer close(forwardComplete)
		for build := range agent.MergedBuildC {
			buildCh <- build
		}
	}()

	callerCleanups := myCleanups
	callerCleanups.add("flush u/", func() error {
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
	callerCleanups.add("butler.Activate", func() error {
		butler.Activate()
		return nil
	})
	myCleanups = nil

	return callerCleanups, nil
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

// downloadInputs downloads CIPD and CAS inputs then updates the build.
func downloadInputs(ctx context.Context, wd, cacheBase string, build *bbpb.Build, buildCh chan<- *bbpb.Build) bool {
	// send clone - we are still mutating `build` in this goroutine.
	defer func() { buildCh <- proto.Clone(build).(*bbpb.Build) }() // Always send back the result build.

	logging.Infof(ctx, "downloading agent inputs")

	if build.Infra.Buildbucket.Agent == nil {
		// Most likely happens in `led get-build` process where it creates from an old build
		// before new Agent field was there.
		build.Infra.Buildbucket.Agent = &bbpb.BuildInfra_Buildbucket_Agent{
			Output: &bbpb.BuildInfra_Buildbucket_Agent_Output{
				Status:          bbpb.Status_FAILURE,
				SummaryMarkdown: "Cannot download agent inputs; Build Agent field is not set",
			},
		}
		build.Output = &bbpb.Build_Output{
			Status:          bbpb.Status_INFRA_FAILURE,
			SummaryMarkdown: "Failed to install user packages for this build",
		}
		return false
	}

	agent := build.Infra.Buildbucket.Agent
	agent.Output = &bbpb.BuildInfra_Buildbucket_Agent_Output{
		Status:        bbpb.Status_STARTED,
		AgentPlatform: platform.CurrentPlatform(),
	}
	// send clone - we are still mutating `build` in this goroutine.
	buildCh <- proto.Clone(build).(*bbpb.Build) // Signal started

	// Encapsulate all the installation logic with a defer to set the
	// TotalDuration. As we add more installation logic (e.g. RBE-CAS),
	// TotalDuration should continue to surround that logic.
	agent = build.Infra.Buildbucket.Agent
	err := func() error {
		start := clock.Now(ctx)
		defer func() {
			agent.Output.TotalDuration = &durationpb.Duration{
				Seconds: int64(clock.Since(ctx, start).Round(time.Second).Seconds()),
			}
		}()
		// Download CIPD.
		if err := installCipd(ctx, build, wd, cacheBase, agent.Output.AgentPlatform); err != nil {
			return err
		}
		if err := prependPath(build, wd); err != nil {
			return err
		}
		if err := installCipdPackages(ctx, build, wd, cacheBase); err != nil {
			return err
		}
		return downloadCasFiles(ctx, build, wd)
	}()
	if err != nil {
		logging.Errorf(ctx, "Failure in installing user packages: %s", err)
		agent.Output.Status = bbpb.Status_FAILURE
		agent.Output.SummaryMarkdown = err.Error()
		build.Output = &bbpb.Build_Output{
			Status:          bbpb.Status_INFRA_FAILURE,
			SummaryMarkdown: "Failed to install user packages for this build",
		}
		return false
	} else {
		agent.Output.Status = bbpb.Status_SUCCESS
		return true
	}
}
