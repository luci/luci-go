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

package runbuild

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"infra/libs/infraenv"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"

	"github.com/golang/protobuf/ptypes"

	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"

	"github.com/golang/protobuf/jsonpb"

	"go.chromium.org/luci/auth/authctx"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/logdog/common/types"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/lucictx"

	pb "go.chromium.org/luci/buildbucket/proto"
)

const (
	systemAccountID    = "system"
	logDogViewerURLTag = "logdog.viewer_url"
	defaultRPCTimeout  = 30 * time.Second
)

type buildRunner struct {
	args         *pb.RunBuildArgs
	localLogFile string
}

// Run runs a user executable and runs it.
func (r *buildRunner) Run(ctx context.Context, args *pb.RunBuildArgs) (*pb.Build, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Validate and normalize parameters.
	args = proto.Clone(args).(*pb.RunBuildArgs)
	if err := normalizeArgs(args); err != nil {
		return nil, errors.Annotate(err, "invalid args").Err()
	}

	// Print our input.
	runBuildJSON, err := indentedJSONPB(args)
	if err != nil {
		return nil, err
	}
	logging.Infof(ctx, "RunBuildArgs: %s", runBuildJSON)

	// Prepare workdir.
	if err := r.setupWorkDir(args.WorkDir); err != nil {
		return nil, err
	}

	// Prepare auth contexts.
	systemAuth, userAuth, err := setupAuth(ctx, args)
	if err != nil {
		return nil, err
	}
	defer systemAuth.Close()
	defer userAuth.Close()

	// Prepare a build listener.
	var listenerErr error
	var listenerErrMU sync.Mutex
	listener := buildListener{
		StreamNamePrefix: "u",
		OnErr: func(err error) {
			logging.Errorf(ctx, "%s", err)

			listenerErrMU.Lock()
			defer listenerErrMU.Unlock()
			if listenerErr == nil {
				listenerErr = err
				logging.Warningf(ctx, "canceling the user subprocess")
				cancel()
			}
		},
	}

	// Start a local LogDog server.
	logdogServ := &logdogServer{
		WorkDir:                    args.WorkDir,
		Authenticator:              systemAuth.Authenticator(),
		CoordinatorHost:            args.LogdogHost,
		Project:                    types.ProjectName(args.Build.Builder.Project),
		Prefix:                     types.StreamName(fmt.Sprintf("buildbucket/%s/%d", args.BuildbucketHost, args.Build.Id)),
		LocalFile:                  r.localLogFile,
		GlobalTags:                 globalLogTags(args),
		StreamRegistrationCallback: listener.StreamRegistrationCallback(),
	}
	if err := logdogServ.Start(ctx); err != nil {
		return nil, errors.Annotate(err, "failed to start local logdog server").Err()
	}
	defer func() {
		if logdogServ != nil {
			logdogServ.Stop()
		}
	}()

	// Start the user subprocess.
	proc, err := r.setupUserSubprocess(ctx, args, userAuth, logdogServ, listener.StreamNamePrefix)
	if err != nil {
		return nil, err
	}
	stdout, stderr, err := proc.Start(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "failed to start the user executable").Err()
	}
	logging.Infof(ctx, "Started user executable successfully")

	// Send subprocess stdout/stderr to logdog.
	if err := r.hookStdoutStderr(ctx, logdogServ, stdout, stderr, listener.StreamNamePrefix); err != nil {
		return nil, err
	}

	// Wait for the user subprocess to exit.
	if err := proc.Wait(ctx); err != nil {
		return nil, err
	}

	// Wait for logdog server to stop before returning the build.
	if err := logdogServ.Stop(); err != nil {
		return nil, errors.Annotate(err, "failed to stop logdog server").Err()
	}
	logdogServ = nil // do not stop for second time

	// Check listener error.
	listenerErrMU.Lock()
	err = listenerErr
	listenerErrMU.Unlock()
	if err != nil {
		return nil, err
	}

	// TODO(nodir): make a final UpdateBuild call.

	return listener.Build, nil
}

// setupWorkDir creates a work dir.
// If workdir already exists, returns an error.
func (r *buildRunner) setupWorkDir(workDir string) error {
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

// setupUserSubprocess prepares a user subprocess.
func (r *buildRunner) setupUserSubprocess(ctx context.Context, args *pb.RunBuildArgs, userAuth *authctx.Context, logdogServ *logdogServer, logdogNamespace string) (*userSubprocess, error) {
	proc := &userSubprocess{
		Path:      args.ExecutablePath,
		InitBuild: args.Build,
	}

	// Prepare user LUCI context.
	ctx, err := lucictx.Set(ctx, "run_build", map[string]string{
		"cache_dir": args.CacheDir,
	})
	if err != nil {
		return nil, err
	}
	userLUCICtx, err := lucictx.ExportInto(ctx, args.WorkDir)
	if err != nil {
		return nil, err
	}

	// Setup user working directory. This is cwd for the user executable itself.
	// Keep it short. This is important to allow tasks on Windows to have as many
	// characters as possible; otherwise they run into MAX_PATH issues.
	proc.Dir = filepath.Join(args.WorkDir, "u")
	if err := os.MkdirAll(proc.Dir, 0700); err != nil {
		return nil, errors.Annotate(err, "failed to create user workdir dir").Err()
	}

	// Prepare user environment.
	proc.Env = environ.System()
	userAuth.ExportIntoEnv(proc.Env)
	logdogServ.ExportIntoEnv(proc.Env)
	proc.Env.Set("LOGDOG_NAMESPACE", logdogNamespace)
	userLUCICtx.SetInEnviron(proc.Env)

	// Prepare a user temp dir.
	// Note that we can't use workdir directly because some overzealous scripts
	// like to remove everything they find under TEMPDIR, and it breaks runbuild
	// internals that keep some files in workdir (in particular git and gsutil
	// configs setup by AuthContext).
	userTempDir := filepath.Join(args.WorkDir, "ut")
	if err := os.MkdirAll(userTempDir, 0700); err != nil {
		return nil, errors.Annotate(err, "failed to create temp dir").Err()
	}
	for _, v := range []string{"TEMPDIR", "TMPDIR", "TEMP", "TMP", "MAC_CHROMIUM_TMPDIR"} {
		proc.Env.Set(v, userTempDir)
	}

	return proc, nil
}

// hookStdoutStderr sends stdout and stderr to logdogServ and also tees
// current process's stdout/stderr accordingly.
func (r *buildRunner) hookStdoutStderr(ctx context.Context, logdogServ *logdogServer, stdout, stderr io.ReadCloser, streamNamePrefix string) error {
	now := clock.Now(ctx)
	tsNow, err := ptypes.TimestampProto(now)
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

	if hook(stdout, "stdout", streamproto.TeeStdout); err != nil {
		return err
	}
	if hook(stderr, "stderr", streamproto.TeeStderr); err != nil {
		return err
	}
	return nil
}

// readBuildSecrets populates c.buildSecrets from swarming secret bytes, if any.
func readBuildSecrets(ctx context.Context) (*pb.BuildSecrets, error) {
	swarming := lucictx.GetSwarming(ctx)
	if swarming == nil {
		return nil, nil
	}

	secrets := &pb.BuildSecrets{}
	if err := proto.Unmarshal(swarming.SecretBytes, secrets); err != nil {
		return nil, err
	}
	return secrets, nil
}

// setupAuth prepares systemAuth and userAuth contexts based on incoming
// environment and command line flags.
//
// Such contexts can be used by runbuild itself or by subprocesses launched by
// Kitchen.
//
// There are two such contexts: a system context and a user context.
//
// The system auth context is used for running logdog and updating Buildbucket
// build state. On Swarming all these actions will use bot-associated account
// (specified in Swarming bot config), whose logical name (usually "system") is
// provided via "-luci-system-account" command-line flag.
//
// The user context is used for actually running the user executable. It is the
// context the runbuild starts with by default. On Swarming this will be the
// context associated with service account specified in the Swarming task
// definition.
func setupAuth(ctx context.Context, args *pb.RunBuildArgs) (system, user *authctx.Context, err error) {
	authArgs := args.GetAuth()

	// Construct authentication option with the set of scopes to be used through
	// out runbuild. This is superset of all scopes we might need. It is more
	// efficient to create a single token with all the scopes than make a bunch
	// of smaller-scoped tokens. We trust Google APIs enough to send
	// widely-scoped tokens to them.
	//
	// Note that the user subprocess are still free to request whatever scopes
	// they need (though LUCI_CONTEXT protocol). The scopes here are only for
	// parts of Kitchen (LogDog client, BigQuery export, Devshell proxy, etc).
	//
	// See https://developers.google.com/identity/protocols/googlescopes for list of
	// available scopes.
	authOpts := infraenv.DefaultAuthOptions()
	authOpts.Scopes = []string{
		"https://www.googleapis.com/auth/cloud-platform",
		"https://www.googleapis.com/auth/userinfo.email",
	}
	if authArgs.GetFirebase() != nil {
		authOpts.Scopes = append(authOpts.Scopes, "https://www.googleapis.com/auth/firebase")
	}

	// If we are given -luci-system-account flag, use the corresponding logical
	// account if it's in the LUCI_CONTEXT (fail if not).
	//
	// Otherwise, we run Kitchen with whatever is default account now (don't
	// switch to a system one). Happens when running Kitchen manually locally. It
	// picks up the developer account.
	systemCtx, err := lucictx.SwitchLocalAccount(ctx, systemAccountID)
	if err != nil {
		return nil, nil, errors.Annotate(err, "failed to prepare system auth context").Err()
	}

	system = &authctx.Context{
		ID:                 "system",
		Options:            authOpts,
		EnableGitAuth:      authArgs.GetGit() != nil,
		EnableDevShell:     authArgs.GetDevShell() != nil,
		EnableDockerAuth:   authArgs.GetDocker() != nil,
		EnableFirebaseAuth: authArgs.GetFirebase() != nil,
		KnownGerritHosts:   authArgs.GetGit().GetKnownGerritHosts(),
	}
	user = new(authctx.Context)
	*user = *system
	user.ID = "task"

	if _, err := system.Launch(systemCtx, args.WorkDir); err != nil {
		return nil, nil, errors.Annotate(err, "failed to start system auth context").Err()
	}
	if _, err := user.Launch(ctx, args.WorkDir); err != nil {
		system.Close() // best effort cleanup
		return nil, nil, errors.Annotate(err, "failed to start user auth context").Err()
	}

	// Log the actual service account emails corresponding to each context.
	system.Report()
	user.Report()
	return
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

func normalizeArgs(a *pb.RunBuildArgs) error {
	switch {
	case a.BuildbucketHost == "":
		return errors.Reason("buildbucket_hostname is required").Err()
	case a.LogdogHost == "":
		return errors.Reason("logdog.host is required").Err()
	case a.Build.GetId() == 0:
		return errors.Reason("build.id is required").Err()
	case a.WorkDir == "":
		return errors.Reason("work_dir is required").Err()
	case a.ExecutablePath == "":
		return errors.Reason("executable_path is required").Err()
	}

	var err error
	a.WorkDir, err = filepath.Abs(a.WorkDir)
	if err != nil {
		return err
	}

	// TODO(nodir): complete.
	return nil
}

// globalLogTags returns tags to be added to all logdog streams by default.
func globalLogTags(args *pb.RunBuildArgs) map[string]string {
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
