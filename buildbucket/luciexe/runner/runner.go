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
	"os"
	"sync"

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

	// Prepare workdir and auth
	if err := setupWorkDir(args.WorkDir); err != nil {
		return err
	}
	authCtx, err := runnerauth.UserServers(ctx, args)
	if err != nil {
		return err
	}
	defer authCtx.Close(ctx)

	cancelCtx, cancelExe := context.WithCancel(ctx)
	defer cancelExe()

	// TODO(iannucci): refactor spy so that:
	//   * it holds its own error
	//   * calls rawCB
	//   * has sane way to send 'final' Build message (so it can 'own' rawCB
	//     entirely and we don't race with ourself)
	spy, spyErr, logdogServ, err := runButler(ctx, args, cancelExe)
	defer func() {
		if logdogServ != nil {
			_ = logdogServ.Stop()
		}
	}()

	// Run the user executable.
	err = runUserExecutable(cancelCtx, args, authCtx, logdogServ, streamNamePrefix)
	if err != nil {
		return err
	}

	// Wait for logdog server to stop before returning the build.
	if err := logdogServ.Stop(); err != nil {
		return errors.Annotate(err, "failed to stop logdog server").Err()
	}
	logdogServ = nil // do not stop for the second time.

	// Check spy error.
	if err = spyErr(); err != nil {
		return err
	}

	// Read the final build state.
	build := spy.Build()
	if build == nil {
		return errors.Reason("user executable did not send a build").Err()
	}
	processFinalBuild(ctx, build)

	// Print the final build state.
	buildJSON, err := indentedJSONPB(build)
	if err != nil {
		return err
	}
	logging.Infof(ctx, "final build state: %s", buildJSON)

	// The final update is critical.
	// If it fails, it is fatal to the build.
	if err := updateBuild(ctx, build, true, rawCB); err != nil {
		return errors.Annotate(err, "final UpdateBuild failed").Err()
	}
	return nil
}

func runButler(ctx context.Context, args *pb.RunnerArgs, cancelExe func()) (*buildspy.Spy, func() error, *runnerbutler.Server, error) {
	systemAuth, err := runnerauth.System(ctx, args)
	if err != nil {
		return nil, nil, nil, err
	}

	// Prepare a build spy.
	var spyErr error
	var spyErrMu sync.Mutex
	spy := buildspy.New(streamNamePrefix, func(err error) {
		logging.Errorf(ctx, "%s", err)

		spyErrMu.Lock()
		defer spyErrMu.Unlock()
		if spyErr == nil {
			spyErr = err
			logging.Warningf(ctx, "canceling the user subprocess")
			cancelExe()
		}
	})
	getErr := func() error {
		spyErrMu.Lock()
		err := spyErr
		spyErrMu.Unlock()
		return err
	}

	// Start a local LogDog server.
	logdogServ, err := startLogDog(ctx, args, systemAuth, spy)
	if err != nil {
		return nil, nil, nil, errors.Annotate(err, "failed to start local logdog server").Err()
	}

	return spy, getErr, logdogServ, nil
}

// setupWorkDir creates a work dir.
// If workdir already exists, returns an error.
func setupWorkDir(workDir string) error {
	switch _, err := os.Stat(workDir); {
	case err == nil:
		return errors.Reason("workdir %q already exists; it must not", workDir).Err()

	case os.IsNotExist(err):
		// good

	default:
		return err
	}

	return errors.Annotate(os.MkdirAll(workDir, 0700), "failed to create %q", workDir).Err()
}

// indentedJSONPB returns m marshaled to indented JSON.
func indentedJSONPB(m proto.Message) ([]byte, error) {
	// Note: json.Indent indents more nicely than jsonpb.Marshaler.
	unindented := &bytes.Buffer{}
	if err := (&jsonpb.Marshaler{}).Marshal(unindented, m); err != nil {
		return nil, err
	}

	indented := &bytes.Buffer{}
	if err := json.Indent(indented, unindented.Bytes(), "", "  "); err != nil {
		return nil, err
	}
	return indented.Bytes(), nil
}
