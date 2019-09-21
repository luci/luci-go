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
)

// Run executes `cb` in a "luciexe" host environment.
//
// The merged Build objects collected from the host environment (i.e. generated
// within `cb`) will be pushed to the returned channel as `cb` executes.
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
func Run(ctx context.Context, options *Options, cb func(context.Context) error) (<-chan *bbpb.Build, error) {
	var opts Options
	if options != nil {
		opts = *options
	}
	if err := opts.initialize(); err != nil {
		return nil, err
	}

	// cleanup will accumulate all of the cleanup functions as we set up the
	// environment. If an error occurs before we can start the user code (`cb`),
	// the defer below will run them all. Otherwise they'll be transferred to the
	// goroutine.
	var cleanup cleanupSlice
	defer cleanup.run()

	cleanupComplete := make(chan struct{})
	cleanup = append(cleanup, func() error {
		close(cleanupComplete)
		return nil
	})

	// First, capture the entire env to restore it later.
	cleanup = append(cleanup, restoreEnv())

	// Startup auth services
	if err := cleanup.add(startAuthServices(ctx, &opts)); err != nil {
		return nil, err
	}

	// Startup butler
	butler, err := startButler(ctx, &opts)
	if err != nil {
		return nil, err
	}
	agent := spyOn(ctx, butler, opts.BaseBuild)
	cleanup = append(cleanup, func() error {
		butler.Activate()
		return butler.Wait()
	})
	cleanup = append(cleanup, func() error {
		agent.Close()
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
	cleanup = nil
	go func() {
		defer userCleanup.run()
		defer func() {
			cctx, cancel := clock.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			logging.Infof(ctx, "waiting up to 30 seconds for user logs to flush")
			leftovers := butler.CloseNamespace(cctx, agent.UserNamespace)
			if len(leftovers) > 0 {
				builder := strings.Builder{}
				for _, leftover := range leftovers {
					builder.WriteString("\n  ")
					builder.WriteString(string(leftover))
				}
				logging.Errorf(
					ctx, "failed to flush the following logs:\n  %s", builder.String())
			}
		}()
		defer butler.Activate()

		// TODO(iannucci): do something with retval of cb
		cb(ctx)
	}()

	return buildCh, nil
}
