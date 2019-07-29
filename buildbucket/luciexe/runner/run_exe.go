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
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"

	"go.chromium.org/luci/auth/authctx"
	"go.chromium.org/luci/buildbucket/luciexe/runner/runnerbutler"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"
	"go.chromium.org/luci/lucictx"

	pb "go.chromium.org/luci/buildbucket/proto"
)

// setupUserEnv prepares user subprocess environment.
func setupUserEnv(ctx context.Context, args *pb.RunnerArgs, authCtx *authctx.Context, logdogServ *runnerbutler.Server, logdogNamespace string) (environ.Env, error) {
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
	abs, err := filepath.Abs(".")
	if err != nil {
		return nil, err
	}
	lctx, err := lucictx.ExportInto(ctx, abs)
	if err != nil {
		return nil, err
	}
	lctx.SetInEnviron(env)

	// Prepare a user temp dir.
	// Note that we can't use workdir directly because some overzealous scripts
	// like to remove everything they find under TEMPDIR, and it breaks LUCI
	// runner internals that keep some files in workdir (in particular git and
	// gsutil configs setup by AuthContext).
	userTempDir, err := filepath.Abs("ut")
	if err != nil {
		return nil, errors.Annotate(err, "failed to get abspath of temp dir").Err()
	}
	if err := os.Mkdir(userTempDir, 0700); err != nil {
		return nil, errors.Annotate(err, "failed to create temp dir").Err()
	}
	for _, v := range []string{"TEMPDIR", "TMPDIR", "TEMP", "TMP", "MAC_CHROMIUM_TMPDIR"} {
		env.Set(v, userTempDir)
	}

	return env, nil
}

// runUserExecutable runs the user executable.
// Requires LogDog server to be running.
// Sends user executable stdout/stderr into logdogServ, with teeing enabled.
func runUserExecutable(ctx context.Context, args *pb.RunnerArgs, authCtx *authctx.Context, logdogServ *runnerbutler.Server, logdogNamespace string) (err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	cmd := exec.CommandContext(ctx, args.ExecutablePath)

	// Prepare user env.
	env, err := setupUserEnv(ctx, args, authCtx, logdogServ, logdogNamespace)
	if err != nil {
		return err
	}
	cmd.Env = env.Sorted()

	// Setup user working directory. This is the CWD for the user executable itself.
	// Keep it short. This is important to allow tasks on Windows to have as many
	// characters as possible; otherwise they run into MAX_PATH issues.
	cmd.Dir = "u"
	if err := os.Mkdir(cmd.Dir, 0700); err != nil {
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
	if err := hookStdoutStderr(ctx, logdogServ, stdout, stderr, streamNamePrefix); err != nil {
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

// hookStdoutStderr sends stdout/stderr to logdogServ and also tees to the
// current process's stdout/stderr respectively.
func hookStdoutStderr(ctx context.Context, logdogServ *runnerbutler.Server, stdout, stderr io.ReadCloser, streamNamePrefix string) error {
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
