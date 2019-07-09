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

package luciexe

import (
	"context"
	"fmt"
	"io"
	"os"

	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/environ"
	grpcLogging "go.chromium.org/luci/grpc/logging"

	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/butler"
	"go.chromium.org/luci/logdog/client/butler/bootstrap"
	"go.chromium.org/luci/logdog/client/butler/bundler"
	"go.chromium.org/luci/logdog/client/butler/output"
	"go.chromium.org/luci/logdog/client/butler/output/file"
	out "go.chromium.org/luci/logdog/client/butler/output/logdog"
	"go.chromium.org/luci/logdog/client/butler/streamserver"
	"go.chromium.org/luci/logdog/common/types"
)

// logdogServer is a LogDog local server (aka butler).
type logdogServer struct {
	WorkDir         string
	Authenticator   *auth.Authenticator
	CoordinatorHost string
	Project         types.ProjectName
	Prefix          types.StreamName
	GlobalTags      map[string]string

	// Write logs to a local file at this path instead of sending to cloud.
	// Overrides Auth, Host, Project and Prefix.
	LocalFile string

	// value for butler.Config.StreamRegistrationCallback. See its docs.
	StreamRegistrationCallback func(*logpb.LogStreamDescriptor) bundler.StreamChunkCallback

	serv   streamserver.StreamServer
	output output.Output
	butler *butler.Butler
}

// Start starts the server.
// Disables gRPC logging.
// The caller is responsible for calling Stop().
func (l *logdogServer) Start(ctx context.Context) error {
	if l.serv != nil {
		return errors.Reason("already started").Err()
	}

	disableGRPCLogging(ctx)

	serv, err := newLogDogStreamServerForPlatform(withNonCancel(ctx), l.WorkDir)
	if err != nil {
		return errors.Annotate(err, "failed to create stream server").Err()
	}

	if err := serv.Listen(); err != nil {
		return errors.Annotate(err, "failed to listen on stream server").Err()
	}
	logging.Debugf(ctx, "Generated stream server at: %s", serv.Address())
	defer func() {
		if err != nil {
			serv.Close()
		}
	}()

	// Create a butler output.
	var output output.Output
	if l.LocalFile != "" {
		// Debug: Use a file output.
		outCfg := file.Options{Path: l.LocalFile}
		output = outCfg.New(ctx)
	} else {
		outCfg := out.Config{
			Auth:           l.Authenticator,
			Host:           l.CoordinatorHost,
			Project:        l.Project,
			Prefix:         l.Prefix,
			SourceInfo:     []string{"luci_runner"},
			RPCTimeout:     defaultRPCTimeout,
			PublishContext: withNonCancel(ctx),
		}
		output, err = outCfg.Register(ctx)
		if err != nil {
			return errors.Annotate(err, "failed to create output instance").Err()
		}
	}
	defer func() {
		if err != nil {
			output.Close()
		}
	}()

	// Create a Butler.
	cfg := butler.Config{
		Output:                     output,
		Project:                    l.Project,
		Prefix:                     l.Prefix,
		BufferLogs:                 true,
		MaxBufferAge:               butler.DefaultMaxBufferAge,
		GlobalTags:                 l.GlobalTags,
		StreamRegistrationCallback: l.StreamRegistrationCallback,
		TeeStdout:                  os.Stdout,
		TeeStderr:                  os.Stderr,
	}

	// Using testclock breaks stopping the butler.
	// Enforce system clock.
	butlerCtx := clock.Set(ctx, clock.GetSystemClock())

	butler, err := butler.New(butlerCtx, cfg)

	if err != nil {
		return err
	}

	butler.AddStreamServer(serv)

	l.serv = serv
	l.output = output
	l.butler = butler
	return nil
}

// SetInEnviron modifies env to export location of this local LogDog server
// into the environment, so a subprocesses can stream logs.
func (l *logdogServer) SetInEnviron(env environ.Env) error {
	if l.serv == nil {
		return fmt.Errorf("not started")
	}

	(&bootstrap.Environment{
		CoordinatorHost: l.CoordinatorHost,
		Project:         l.Project,
		Prefix:          l.Prefix,
		StreamServerURI: l.serv.Address(),
	}).Augment(env)
	return nil
}

// AddStream adds a new stream.
// If no error is returned, the logdogServ assumes ownership of the supplied
// stream. The stream will be closed when processing is finished.
func (l *logdogServer) AddStream(rc io.ReadCloser, props *streamproto.Properties) error {
	return l.butler.AddStream(rc, props)
}

// Stop stops the server.
func (l *logdogServer) Stop() error {
	if l.serv == nil {
		return fmt.Errorf("not started")
	}

	l.butler.Activate()
	if err := l.butler.Wait(); err != nil {
		return err
	}

	l.output.Close()

	l.serv = nil
	l.output = nil
	l.butler = nil
	return nil
}

// disableGRPCLogging routes gRPC log messages that are emitted through our
// logger. We only log gRPC prints if our logger is configured to log
// debug-level or lower, which it isn't by default.
func disableGRPCLogging(ctx context.Context) {
	level := logging.Debug
	if !logging.IsLogging(ctx, logging.Debug) {
		level = grpcLogging.Suppress
	}
	grpcLogging.Install(logging.Get(ctx), level)
}
