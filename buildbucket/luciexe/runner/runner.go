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

package runner

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/buildbucket/luciexe/runner/buildspy"
	"go.chromium.org/luci/buildbucket/luciexe/runner/runnerauth"
	"go.chromium.org/luci/buildbucket/luciexe/runner/runnerbutler"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	pb "go.chromium.org/luci/buildbucket/proto"
)

const (
	streamNamePrefix = "u"
)

// updateBuildCB is periodically called with the latest state of the build and
// the list field paths that have changes.
// Should return a GRPC error, e.g. status.Errorf. The error MAY be wrapped
// with errors.Annotate.
type updateBuildCB func(context.Context, *pb.UpdateBuildRequest) error

// run runs a user executable and periodically calls rawCB with the
// latest state of the build.
// Calls rawCB sequentially.
//
// If rawCB is nil, panics.
// Users are expected to initialize rawCB at least to read the latest
// state of the build.
func run(ctx context.Context, args *pb.RunnerArgs, rawCB updateBuildCB) error {
	if rawCB == nil {
		panic("rawCB is nil")
	}

	// Prepare auth
	authCtx, err := runnerauth.UserServers(ctx, args)
	if err != nil {
		return err
	}
	defer authCtx.Close(ctx)

	cancelCtx, cancelExe := context.WithCancel(ctx)
	defer cancelExe()

	spy, logdogServ, err := runButler(ctx, args, cancelExe)
	defer func() {
		if lerr := logdogServ.Stop(); lerr != nil {
			logging.WithError(err).Errorf(ctx, "failed to stop logdog server")
		}
	}()

	buildChan, closewait := runBuildDispatcher(ctx, logdogServ, rawCB)

	var lastBuild *pb.Build
	spyDone := make(chan struct{})
	go func() {
		defer close(spyDone)

		for spyData := range spy.C {
			if spyData.Err != nil {
				logging.Errorf(ctx, "%s", err)
				logging.Warningf(ctx, "canceling the user subprocess")
				cancelExe()
				break
			}

			lastBuild = spyData.Build
			buildChan <- spyData.Build
		}
		// This should be a no-op, but sink spy.C just in case.
		for range spy.C {
			logging.Errorf(ctx, "got extra build.proto?")
		}
	}()

	// Run the user executable.
	err = runUserExecutable(cancelCtx, args, authCtx, logdogServ, streamNamePrefix)
	if err != nil {
		return err
	}

	// Wait for spy to drain. This should occur automatically even without
	// stopping the logdog server, since the build.proto pipe will have been
	// closed when the user executable terminated.
	//
	// TODO(iannucci): currently this blocks for up to 5 seconds to let butler's
	// internal buffer age out. I should rework the butler callbacks so they're
	// not tied to the internal batching mechanisms of butler.
	logging.Infof(ctx, "waiting for final build.proto")
	<-spyDone
	logging.Infof(ctx, "got final build.proto")

	if lastBuild == nil {
		return errors.Reason("user executable did not send a build").Err()
	}
	processFinalBuild(ctx, lastBuild)

	logging.Infof(ctx, "final build state: %s", indentedJSONPB(lastBuild))

	buildChan <- lastBuild
	logging.Infof(ctx, "waiting for build dispatcher to flush")
	return errors.Annotate(closewait(ctx), "failed to send last build update").Err()
}

func runButler(ctx context.Context, args *pb.RunnerArgs, cancelExe func()) (*buildspy.Spy, *runnerbutler.Server, error) {
	systemAuth, err := runnerauth.System(ctx, args)
	if err != nil {
		return nil, nil, err
	}

	// Start a local LogDog server.
	logdogServ, err := makeButler(ctx, args, systemAuth)
	if err != nil {
		return nil, nil, errors.Annotate(err, "failed to create local logdog server").Err()
	}

	// Install the spy before we start the server.
	spy := buildspy.New(streamNamePrefix)
	spy.On(logdogServ)
	if err := logdogServ.Start(ctx); err != nil {
		return nil, nil, errors.Annotate(err, "failed to start local logdog server").Err()
	}

	return spy, logdogServ, nil
}

// indentedJSONPB returns m marshaled to indented JSON.
//
// panics on failure
func indentedJSONPB(m proto.Message) []byte {
	// Note: json.Indent indents more nicely than jsonpb.Marshaler.
	unindented := &bytes.Buffer{}
	if err := (&jsonpb.Marshaler{}).Marshal(unindented, m); err != nil {
		panic(err)
	}

	indented := &bytes.Buffer{}
	if err := json.Indent(indented, unindented.Bytes(), "", "  "); err != nil {
		panic(err)
	}
	return indented.Bytes()
}
