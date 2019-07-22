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
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sync"
	"time"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/authctx"
	"go.chromium.org/luci/buildbucket/luciexe/runner/buildspy"
	"go.chromium.org/luci/buildbucket/luciexe/runner/runnerauth"
	"go.chromium.org/luci/buildbucket/luciexe/runner/runnerbutler"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"
	"go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/lucictx"

	pb "go.chromium.org/luci/buildbucket/proto"
)

const (
	logDogViewerURLTag = "logdog.viewer_url"
	streamNamePrefix   = "u"
)

// runner runs a LUCI executable.
type runner struct {
	// UpdateBuild is periodically called with the latest state of the build and
	// the list field paths that have changes.
	// Should return a GRPC error, e.g. status.Errorf. The error MAY be wrapped
	// with errors.Annotate.
	UpdateBuild func(context.Context, *pb.UpdateBuildRequest) error

	localLogFile string

	testExtraArgs []string // extra args to pass to the executable, for tests
	testExtraEnv  []string // extra environ to pass to the executable, for tests
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

	// Validate and normalize parameters.
	args = proto.Clone(args).(*pb.RunnerArgs)
	if err := normalizeArgs(args); err != nil {
		return errors.Annotate(err, "invalid args").Err()
	}

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

// runUserExecutable runs the user executable.
// Requires LogDog server to be running.
// Sends user executable stdout/stderr into logdogServ, with teeing enabled.
func (r *runner) runUserExecutable(ctx context.Context, args *pb.RunnerArgs, authCtx *authctx.Context, logdogServ *runnerbutler.Server, logdogNamespace string) (err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	cmd := exec.CommandContext(ctx, args.ExecutablePath, r.testExtraArgs...)

	// Prepare user env.
	env, err := r.setupUserEnv(ctx, args, authCtx, logdogServ, logdogNamespace)
	if err != nil {
		return err
	}
	cmd.Env = append(env.Sorted(), r.testExtraEnv...)

	// Setup user working directory. This is the CWD for the user executable itself.
	// Keep it short. This is important to allow tasks on Windows to have as many
	// characters as possible; otherwise they run into MAX_PATH issues.
	cmd.Dir = filepath.Join(args.WorkDir, "u")
	if err := os.MkdirAll(cmd.Dir, 0700); err != nil {
		return errors.Annotate(err, "failed to create user workdir dir at %q", cmd.Dir).Err()
	}

	// Pass initial build on stdin.
	buildBytes, err := proto.Marshal(args.Build)
	if err != nil {
		return
	}
	cmd.Stdin = bytes.NewReader(buildBytes)

	// Prepare stdout/stderr pipes.
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return
	}

	// Start the user executable.
	if err := cmd.Start(); err != nil {
		return errors.Annotate(err, "failed to start the user executable").Err()
	}
	logging.Infof(ctx, "Started user executable successfully")

	// Send subprocess stdout/stderr to logdog.
	if err := r.hookStdoutStderr(ctx, logdogServ, stdout, stderr, streamNamePrefix); err != nil {
		return err
	}

	switch err := cmd.Wait().(type) {
	case *exec.ExitError:
		// The subprocess exited with a non-zero exit code.
		// This is OK. Exit code is not a part of user executable contract.
		logging.Infof(ctx, "user subprocess exited with code %d", err.ExitCode())
		return nil

	case nil:
		logging.Infof(ctx, "user subprocess has exited with zero code")
		return nil

	default:
		return errors.Annotate(err, "failed to wait for the user executable to exit").Err()
	}
}

// setupUserEnv prepares user subprocess environment.
func (r *runner) setupUserEnv(ctx context.Context, args *pb.RunnerArgs, authCtx *authctx.Context, logdogServ *runnerbutler.Server, logdogNamespace string) (environ.Env, error) {
	env := environ.System()
	ctx = authCtx.Export(ctx, env)
	if err := logdogServ.SetInEnviron(env); err != nil {
		return nil, err
	}
	env.Set("LOGDOG_NAMESPACE", logdogNamespace)

	// Prepare user LUCI context.
	ctx, err := lucictx.Set(ctx, "luci_executable", map[string]string{
		"cache_dir": args.CacheDir,
	})
	if err != nil {
		return nil, err
	}
	lctx, err := lucictx.ExportInto(ctx, args.WorkDir)
	if err != nil {
		return nil, err
	}
	lctx.SetInEnviron(env)

	// Prepare a user temp dir.
	// Note that we can't use workdir directly because some overzealous scripts
	// like to remove everything they find under TEMPDIR, and it breaks LUCI
	// runner internals that keep some files in workdir (in particular git and
	// gsutil configs setup by AuthContext).
	userTempDir := filepath.Join(args.WorkDir, "ut")
	if err := os.MkdirAll(userTempDir, 0700); err != nil {
		return nil, errors.Annotate(err, "failed to create temp dir").Err()
	}
	for _, v := range []string{"TEMPDIR", "TMPDIR", "TEMP", "TMP", "MAC_CHROMIUM_TMPDIR"} {
		env.Set(v, userTempDir)
	}

	return env, nil
}

// hookStdoutStderr sends stdout/stderr to logdogServ and also tees to the
// current process's stdout/stderr respectively.
func (r *runner) hookStdoutStderr(ctx context.Context, logdogServ *runnerbutler.Server, stdout, stderr io.ReadCloser, streamNamePrefix string) error {
	tsNow, err := ptypes.TimestampProto(clock.Now(ctx))
	if err != nil {
		return err
	}

	hook := func(rc io.ReadCloser, name string, tee streamproto.TeeType) error {
		return logdogServ.AddStream(rc, &streamproto.Properties{
			LogStreamDescriptor: &logpb.LogStreamDescriptor{
				Name:        path.Join(streamNamePrefix, name),
				ContentType: "text/plain",
				Timestamp:   tsNow,
			},
			Tee: tee,
		})
	}

	if err := hook(stdout, "stdout", streamproto.TeeStdout); err != nil {
		return err
	}
	if err := hook(stderr, "stderr", streamproto.TeeStderr); err != nil {
		return err
	}
	return nil
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

func normalizePath(title, path string) (string, error) {
	if path == "" {
		return "", errors.Reason("%s is required", title).Err()
	}
	return filepath.Abs(path)
}

func normalizeArgs(a *pb.RunnerArgs) error {
	switch {
	case a.BuildbucketHost == "":
		return errors.Reason("buildbucket_host is required").Err()
	case a.LogdogHost == "":
		return errors.Reason("logdog_host is required").Err()
	case a.Build.GetId() == 0:
		return errors.Reason("build.id is required").Err()
	}

	var err error
	if a.WorkDir, err = normalizePath("work_dir", a.WorkDir); err != nil {
		return err
	}
	if a.ExecutablePath, err = normalizePath("executable_path", a.ExecutablePath); err != nil {
		return err
	}
	if a.CacheDir, err = normalizePath("cache_dir", a.CacheDir); err != nil {
		return err
	}
	return nil
}

func (r *runner) startLogDog(ctx context.Context, args *pb.RunnerArgs, systemAuth *auth.Authenticator, spy *buildspy.Spy) (*runnerbutler.Server, error) {
	logdogServ := &runnerbutler.Server{
		WorkDir:                    args.WorkDir,
		Authenticator:              systemAuth,
		CoordinatorHost:            args.LogdogHost,
		Project:                    types.ProjectName(args.Build.Builder.Project),
		Prefix:                     types.StreamName(fmt.Sprintf("buildbucket/%s/%d", args.BuildbucketHost, args.Build.Id)),
		LocalFile:                  r.localLogFile,
		GlobalTags:                 globalLogTags(args),
		StreamRegistrationCallback: spy.StreamRegistrationCallback,
	}
	return logdogServ, logdogServ.Start(ctx)
}

// globalLogTags returns tags to be added to all logdog streams by default.
func globalLogTags(args *pb.RunnerArgs) map[string]string {
	ret := make(map[string]string, 4)
	ret[logDogViewerURLTag] = fmt.Sprintf("https://%s/build/%d", args.BuildbucketHost, args.Build.Id)

	// SWARMING_SERVER is the full URL: https://example.com
	// We want just the hostname.
	env := environ.System()
	if v, ok := env.Get("SWARMING_SERVER"); ok {
		if u, err := url.Parse(v); err == nil && u.Host != "" {
			ret["swarming.host"] = u.Host
		}
	}
	if v, ok := env.Get("SWARMING_TASK_ID"); ok {
		ret["swarming.run_id"] = v
	}
	if v, ok := env.Get("SWARMING_BOT_ID"); ok {
		ret["swarming.bot_id"] = v
	}
	return ret
}

// processFinalBuild adjusts the final state of the build if needed.
func processFinalBuild(ctx context.Context, build *pb.Build) {
	if err := validateFinalBuildState(build); err != nil {
		build.SummaryMarkdown = fmt.Sprintf("Invalid final build state: `%s`. Marking as `INFRA_FAILURE`.", err)
		build.Status = pb.Status_INFRA_FAILURE
	}

	nowTS, err := ptypes.TimestampProto(clock.Now(ctx))
	if err != nil {
		panic(err)
	}

	// Mark incomplete steps as canceled.
	for _, s := range build.Steps {
		if !protoutil.IsEnded(s.Status) {
			s.Status = pb.Status_CANCELED
			if s.SummaryMarkdown != "" {
				s.SummaryMarkdown += "\n"
			}
			s.SummaryMarkdown += "step was canceled because it did not end before build ended"
			s.EndTime = nowTS
		}
	}
}

// validateFinalBuildState validates the build after LUCI executable
// finished.
func validateFinalBuildState(build *pb.Build) error {
	if !protoutil.IsEnded(build.Status) {
		return fmt.Errorf("expected a terminal build status, got %s", build.Status)
	}
	return nil
}

// updateBuild calls r.UpdateBuild.
// If final is true, may update the build status, making it immutable.
//
// Final calls will retry with exponential backoff for up to 5 minutes.
// Non-final calls will do NO retries.
//
// May return a transient error (in the event that the final RPC was actually
// a retryable error).
func (r *runner) updateBuild(ctx context.Context, build *pb.Build, final bool) error {
	req := &pb.UpdateBuildRequest{
		Build: build,
		UpdateMask: &field_mask.FieldMask{
			Paths: []string{
				"build.steps",
				"build.output.properties",
				"build.output.gitiles_commit",
				"build.summary_markdown",
			},
		},
	}

	if final {
		// If the build has failed, update the build status.
		// If it succeeded, do not set it just yet, since there are more ways
		// the build can fail.
		switch {
		case !protoutil.IsEnded(build.Status):
			return fmt.Errorf("build status %q is not final", build.Status)
		case build.Status != pb.Status_SUCCESS:
			req.UpdateMask.Paths = append(req.UpdateMask.Paths, "build.status")
		}
	}

	// Make the RPC.
	//
	// If this is the final update then we use a retry iterator to do our best to
	// ensure the final build message goes through.
	retryFactory := retry.None
	if final {
		retryFactory = func() retry.Iterator {
			return &retry.ExponentialBackoff{
				Limited: retry.Limited{
					Retries:  -1, // no limit
					MaxTotal: 5 * time.Minute,
				},
				Multiplier: 1.2,
				MaxDelay:   30 * time.Second,
			}
		}
	}

	return retry.Retry(
		ctx, retryFactory,
		func() error {
			err := r.UpdateBuild(ctx, req)
			switch status.Code(errors.Unwrap(err)) {
			case codes.OK:
				return nil

			case codes.InvalidArgument:
				// This is fatal.
				return err

			default:
				return transient.Tag.Apply(err)
			}
		},
		retry.LogCallback(ctx, "luciexe.runner.updateBuild"),
	)
}
