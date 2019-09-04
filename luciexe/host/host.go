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
	"os"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/lucictx"
)

type cleanupSlice []func() error

func (c cleanupSlice) run() {
	merr := errors.NewLazyMultiError(len(c))
	for i, fn := range c {
		merr.Assign(i, fn())
	}
	if err := merr.Get(); err != nil {
		panic(err)
	}
}

func (c *cleanupSlice) add(cleanup cleanupSlice, err error) error {
	*c = append(*c, cleanup...)
	return err
}

func startAuthServices(ctx context.Context, opts *Options) (cleanupSlice, error) {
	var envCleanups cleanupSlice
	defer envCleanups.run()

	env := environ.New(nil)

	if err := opts.ExeAuth.Launch(ctx, opts.authDir); err != nil {
		return nil, errors.Annotate(err, "setting up task auth").Err()
	}
	opts.ExeAuth.Report(ctx)
	ctx = opts.ExeAuth.Export(ctx, env)
	envCleanups = append(envCleanups, func() error {
		opts.ExeAuth.Close(ctx)
		return nil
	})

	exported, err := lucictx.ExportInto(ctx, opts.lucictxDir)
	if err != nil {
		return nil, errors.Annotate(err, "exporting LUCI_CONTEXT").Err()
	}
	envCleanups = append(envCleanups, exported.Close)
	exported.SetInEnviron(env)

	if err := env.Iter(os.Setenv); err != nil {
		return nil, errors.Annotate(err, "setting up environment").Err()
	}

	retCleanups := envCleanups
	envCleanups = nil

	return retCleanups, nil
}

func restoreEnv() func() error {
	origEnv := environ.System()
	return func() error {
		os.Clearenv()
		return errors.Annotate(origEnv.Iter(os.Setenv), "restoring original env").Err()
	}
}

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
// running. Be careful when using Run concurrently with other code.
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

	// First, capture the entire env to restore it later.
	cleanup = append(cleanup, restoreEnv())

	// Startup auth services
	if err := cleanup.add(startAuthServices(ctx, &opts)); err != nil {
		return nil, err
	}

	// TODO(iannucci): implement logdog butler
	builds := make(chan *bbpb.Build)

	// Transfer ownership of cleanups to goroutine
	userCleanup := cleanup
	cleanup = nil
	go func() {
		defer close(builds)
		defer userCleanup.run()

		// TODO(iannucci): do something with retval of cb
		cb(ctx)
	}()

	return builds, nil
}
