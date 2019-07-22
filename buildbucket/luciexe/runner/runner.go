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
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	pb "go.chromium.org/luci/buildbucket/proto"
)

const (
	streamNamePrefix = "u"
)

// runner runs a LUCI executable.
type runner struct {
	// UpdateBuild is periodically called with the latest state of the build and
	// the list field paths that have changes.
	// Should return a GRPC error, e.g. status.Errorf. The error MAY be wrapped
	// with errors.Annotate.
	UpdateBuild func(context.Context, *pb.UpdateBuildRequest) error

	localLogFile string
}

// Run runs a user executable and periodically calls r.UpdateBuild with the
// latest state of the build.
// Calls r.UpdateBuild sequentially.
//
// If r.UpdateBuild is nil, panics.
// Users are expected to initialize r.UpdateBuild at least to read the latest
// state of the build.
func (r *runner) Run(ctx context.Context, args *pb.RunnerArgs) error {
	if r.UpdateBuild == nil {
		panic("r.UpdateBuild is nil")
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Print our input.
	argsJSON, err := indentedJSONPB(args)
	if err != nil {
		return err
	}
	logging.Infof(ctx, "RunnerArgs: %s", argsJSON)

	// Prepare workdir.
	if err := r.setupWorkDir(args.WorkDir); err != nil {
		return err
	}

	// Prepare authenticator used by luciexe internals (Logdog, Buildbucket).
	// It is not exported outside the process in any way.
	systemAuth, err := runnerauth.System(ctx, args)
	if err != nil {
		return err
	}

	// Prepare auth context used by the user executable. This launches a ton of
	// various local token server for various third party tools.
	authCtx, err := runnerauth.UserServers(ctx, args)
	if err != nil {
		return err
	}
	defer authCtx.Close(ctx)

	// Prepare a build listener.
	var listenerErr error
	var listenerErrMU sync.Mutex
	listener := buildspy.New(streamNamePrefix, func(err error) {
		logging.Errorf(ctx, "%s", err)

		listenerErrMU.Lock()
		defer listenerErrMU.Unlock()
		if listenerErr == nil {
			listenerErr = err
			logging.Warningf(ctx, "canceling the user subprocess")
			cancel()
		}
	})

	// Start a local LogDog server.
	logdogServ, err := r.startLogDog(ctx, args, systemAuth, listener)
	if err != nil {
		return errors.Annotate(err, "failed to start local logdog server").Err()
	}
	defer func() {
		if logdogServ != nil {
			_ = logdogServ.Stop()
		}
	}()

	// Run the user executable.
	err = r.runUserExecutable(ctx, args, authCtx, logdogServ, streamNamePrefix)
	if err != nil {
		return err
	}

	// Wait for logdog server to stop before returning the build.
	if err := logdogServ.Stop(); err != nil {
		return errors.Annotate(err, "failed to stop logdog server").Err()
	}
	logdogServ = nil // do not stop for the second time.

	// Check listener error.
	listenerErrMU.Lock()
	err = listenerErr
	listenerErrMU.Unlock()
	if err != nil {
		return err
	}

	// Read the final build state.
	build := listener.Build()
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
	if err := r.updateBuild(ctx, build, true); err != nil {
		return errors.Annotate(err, "final UpdateBuild failed").Err()
	}
	return nil
}

// setupWorkDir creates a work dir.
// If workdir already exists, returns an error.
func (r *runner) setupWorkDir(workDir string) error {
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
