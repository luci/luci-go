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
	"os/exec"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/auth/authctx"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
	"go.chromium.org/luci/lucictx"
	"go.chromium.org/luci/luciexe/runner/runnerbutler"

	pb "go.chromium.org/luci/buildbucket/proto"
)

// setupUserEnv prepares user subprocess environment.
func setupUserEnv(ctx context.Context, args *pb.RunnerArgs, wkDir workdir, authCtx *authctx.Context, logdogServ *runnerbutler.Server, logdogNamespace string) (environ.Env, error) {
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
	lctx, err := lucictx.ExportInto(ctx, wkDir.luciCtxDir)
	if err != nil {
		return nil, err
	}
	lctx.SetInEnviron(env)

	for _, v := range []string{"TEMPDIR", "TMPDIR", "TEMP", "TMP", "MAC_CHROMIUM_TMPDIR"} {
		env.Set(v, wkDir.userTempDir)
	}

	return env, nil
}

// runUserExecutable runs the user executable.
// Requires LogDog server to be running.
// Sends user executable stdout/stderr into logdogServ, with teeing enabled.
func runUserExecutable(ctx context.Context, args *pb.RunnerArgs, wkDir workdir, authCtx *authctx.Context, logdogServ *runnerbutler.Server, logdogNamespace string) (err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	logging.Infof(ctx, "prepping to run executable")

	cmd := exec.CommandContext(ctx, args.ExecutablePath)

	// Prepare user env.
	env, err := setupUserEnv(ctx, args, wkDir, authCtx, logdogServ, logdogNamespace)
	if err != nil {
		logging.WithError(err).Errorf(ctx, "failed to setupUserEnv")
		return err
	}
	cmd.Env = env.Sorted()
	cmd.Dir = wkDir.userDir

	// Pass initial build on stdin.
	buildBytes, err := proto.Marshal(args.Build)
	if err != nil {
		logging.WithError(err).Errorf(ctx, "failed to marshal build")
		return
	}
	cmd.Stdin = bytes.NewReader(buildBytes)

	stdout, err := logdogServ.Client.NewTextStream(
		ctx, "stdout", streamclient.ForProcess())
	if err != nil {
		logging.WithError(err).Errorf(ctx, "failed to get stdout")
		return
	}
	defer stdout.Close()
	cmd.Stdout = stdout

	stderr, err := logdogServ.Client.NewTextStream(
		ctx, "stderr", streamclient.ForProcess())
	if err != nil {
		logging.WithError(err).Errorf(ctx, "failed to get stderr")
		return
	}
	defer stderr.Close()
	cmd.Stderr = stderr

	// Start the user executable.
	err = cmd.Start()
	if err != nil {
		return errors.Annotate(err, "failed to start the user executable").Err()
	}
	logging.Infof(ctx, "Started user executable successfully")

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
		logging.WithError(err).Errorf(ctx, "failed to wait")
		return errors.Annotate(err, "failed to wait for the user executable to exit").Err()
	}
}
