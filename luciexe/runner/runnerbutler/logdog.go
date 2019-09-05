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

package runnerbutler

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"time"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/environ"
	grpcLogging "go.chromium.org/luci/grpc/logging"

	"go.chromium.org/luci/logdog/client/butler"
	"go.chromium.org/luci/logdog/client/butler/output"
	out "go.chromium.org/luci/logdog/client/butler/output/logdog"
	"go.chromium.org/luci/logdog/client/butler/output/null"
	"go.chromium.org/luci/logdog/client/butler/streamserver"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
	"go.chromium.org/luci/logdog/common/types"
)

// Server is a LogDog local server (aka butler).
type Server struct {
	WorkDir       string
	Authenticator *auth.Authenticator

	// If CoordinatorHost is "null", this provides a null sink for butler.
	CoordinatorHost string
	Project         string
	Prefix          types.StreamName
	GlobalTags      map[string]string

	serv   *streamserver.StreamServer
	output output.Output
	Butler *butler.Butler

	Client *streamclient.Client
}

// Start starts the server.
// Disables gRPC logging.
// The caller is responsible for calling Stop().
func (l *Server) Start(ctx context.Context) error {
	if l.serv != nil {
		return errors.Reason("already started").Err()
	}

	// TODO(iannucci): Move this to 'main'; this modifies the global grpc logging
	// for the entire process.
	disableGRPCLogging(ctx)

	var streamServerBasePath string
	if runtime.GOOS == "windows" {
		streamServerBasePath = "luciexe_runner"
	} else {
		streamServerBasePath = filepath.Join(l.WorkDir, "ld.sock")
	}

	serv, err := streamserver.New(ctx, streamServerBasePath)
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
	if l.CoordinatorHost == "null" {
		output = &null.Output{}
	} else {
		outCfg := out.Config{
			Auth:           l.Authenticator,
			Host:           l.CoordinatorHost,
			Project:        l.Project,
			Prefix:         l.Prefix,
			SourceInfo:     []string{"luci_runner"},
			RPCTimeout:     30 * time.Second,
			PublishContext: ctx,
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
		Output:       output,
		BufferLogs:   true,
		MaxBufferAge: butler.DefaultMaxBufferAge,
		GlobalTags:   l.GlobalTags,
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
	l.Butler = butler
	l.Client = streamclient.NewLoopback(butler, "")
	return nil
}

// SetInEnviron modifies env to export location of this local LogDog server
// into the environment, so a subprocesses can stream logs.
func (l *Server) SetInEnviron(env environ.Env) error {
	if l.serv == nil {
		return fmt.Errorf("not started")
	}
	ret := l.output.URLConstructionEnv()
	ret.StreamServerURI = l.serv.Address()
	ret.Augment(env)
	return nil
}

// Stop stops the server.
func (l *Server) Stop() error {
	if l.serv == nil {
		return fmt.Errorf("not started")
	}

	l.Butler.Activate()
	if err := l.Butler.Wait(); err != nil {
		return err
	}

	l.output.Close()

	l.serv = nil
	l.output = nil
	l.Butler = nil
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
