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

// Package authctx allows to run subprocesses in an environment with ambient
// auth.
//
// Supports setting up an auth context for LUCI tools, gsutil and gcloud, Git,
// Docker and Firebase.
//
// Git auth depends on presence of Git wrapper and git-credential-luci in PATH.
// Docker auth depends on presence of docker-credential-luci in PATH.
package authctx

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/environ"

	"go.chromium.org/luci/lucictx"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/integration/devshell"
	"go.chromium.org/luci/auth/integration/firebase"
	"go.chromium.org/luci/auth/integration/gsutil"
	"go.chromium.org/luci/auth/integration/localauth"
)

// Context knows how to prepare an environment with ambient authentication for
// various tools: LUCI, gsutil, Docker, Git, Firebase.
//
// 'Launch' launches a bunch of local HTTP servers and writes a bunch of
// configuration files that point to these servers. 'ExportIntoEnv' then exposes
// location of these configuration files to subprocesses, so they can discover
// local HTTP servers and use them to mint tokens.
type Context struct {
	// ID is used in logs, filenames and in LUCI_CONTEXT (if we launch a new one).
	//
	// Usually a logical account name associated with this context, e.g. "task" or
	// "system".
	ID string

	// Options define how to build the root authenticator.
	//
	// This authenticator (perhaps indirectly through LUCI_CONTEXT created in
	// 'Launch') will be used by all other auth helpers to grab access tokens.
	//
	// If Options.Method is LUCIContextMethod, indicating there's some existing
	// LUCI_CONTEXT with "local_auth" section we should use, and service account
	// impersonation is not requested (Options.ActAsServiceAccount == "") the
	// existing LUCI_CONTEXT is reused. Otherwise launches a new local_auth server
	// (that uses given auth options to mint tokens) and puts its location into
	// the new LUCI_CONTEXT. Either way, subprocesses launched with an environment
	// modified by 'ExportIntoEnv' will see a functional LUCI_CONTEXT.
	//
	// When reusing an existing LUCI_CONTEXT, subprocesses inherit all OAuth
	// scopes permissible there.
	Options auth.Options

	// EnableGitAuth enables authentication for Git subprocesses.
	//
	// Assumes 'git' binary is actually gitwrapper and that 'git-credential-luci'
	// binary is in PATH.
	//
	// Requires "https://www.googleapis.com/auth/gerritcodereview" OAuth scope.
	EnableGitAuth bool

	// EnableDockerAuth enables authentication for Docker.
	//
	// Assumes 'docker-credential-luci' is in PATH.
	//
	// Requires Google Storage OAuth scopes. See GCR docs for more info.
	EnableDockerAuth bool

	// EnableDevShell enables DevShell server and gsutil auth shim.
	//
	// They are used to make gsutil and gcloud use LUCI authentication.
	//
	// On Windows only gsutil auth shim is enabled, since enabling DevShell there
	// triggers bugs in gsutil. See https://crbug.com/788058#c14.
	//
	// Requires Google Storage OAuth scopes. See GS docs for more info.
	EnableDevShell bool

	// EnableFirebaseAuth enables Firebase auth shim.
	//
	// It is used to make Firebase use LUCI authentication.
	//
	// Requires "https://www.googleapis.com/auth/firebase" OAuth scope.
	EnableFirebaseAuth bool

	// KnownGerritHosts is list of Gerrit hosts to force git authentication for.
	//
	// By default public hosts are accessed anonymously, and the anonymous access
	// has very low quota. Context needs to know all such hostnames in advance to
	// be able to force authenticated access to them.
	KnownGerritHosts []string

	ctx           context.Context     // stores new LUCI_CONTEXT
	exported      lucictx.Exported    // exported LUCI_CONTEXT on the disk
	tmpDir        string              // non empty if we created a new temp dir
	authenticator *auth.Authenticator // used by in-process helpers
	anonymous     bool                // true if not associated with any account
	email         string              // an account email or "" for anonymous
	luciSrv       *localauth.Server   // non-nil if we launched a LUCI_CONTEXT subcontext

	gitHome string // custom HOME for git or "" if not using git auth

	dockerConfig string // location for Docker configuration files
	dockerTmpDir string // location for Docker temporary files

	gsutilSrv   *gsutil.Server // gsutil auth shim server
	gsutilState string         // path to a context-managed state directory
	gsutilBoto  string         // path to a generated .boto file

	devShellSrv  *devshell.Server // DevShell server instance
	devShellAddr *net.TCPAddr     // address local DevShell instance is listening on

	firebaseSrv      *firebase.Server // firebase auth shim server
	firebaseTokenURL string           // URL to get firebase auth token from
}

// Launch launches this auth context. It must be called before any other method.
//
// It launches various local server and prepares various configs, by putting
// them into tempDir which may be "" to use some new ioutil.TempDir.
//
// On success returns a new context.Context with updated LUCI_CONTEXT value.
//
// To run a subprocess within this new context use 'ExportIntoEnv' to modify an
// environ for a new process.
func (ac *Context) Launch(ctx context.Context, tempDir string) (nc context.Context, err error) {
	defer func() {
		if err != nil {
			ac.Close()
		}
		nc = ac.ctx // nil if err != nil
	}()

	ac.ctx = ctx

	if tempDir == "" {
		ac.tmpDir, err = ioutil.TempDir("", "luci")
		if err != nil {
			return nil, errors.Annotate(err, "failed to create a temp directory").Err()
		}
		tempDir = ac.tmpDir
	}

	// Expand AuthSelectMethod into the actual method. We'll be checking it later.
	// We do this expansion now to consistently use new 'opts' through out.
	opts := ac.Options
	if opts.Method == auth.AutoSelectMethod {
		opts.Method = auth.SelectBestMethod(ac.ctx, opts)
	}

	// Construct the authenticator to be used directly by the helpers hosted in
	// the current process (devshell, gsutil, firebase) and by the new
	// localauth.Server (if we are going to launch it). Out-of-process helpers
	// (git, docker) will use LUCI_CONTEXT protocol.
	ac.authenticator = auth.NewAuthenticator(ac.ctx, auth.SilentLogin, opts)

	// Figure out what email is associated with this account (if any).
	ac.email, err = ac.authenticator.GetEmail()
	switch {
	case err == auth.ErrLoginRequired:
		// This context is not associated with any account. This happens when
		// running Swarming tasks without service account specified or running
		// locally without doing 'luci-auth login' first.
		ac.anonymous = true
	case err != nil:
		return nil, errors.Annotate(err, "failed to get email of %q account", ac.ID).Err()
	}

	// Check whether we are allowed to inherit the existing LUCI_CONTEXT. We do it
	// if 'opts' indicate to use LUCI_CONTEXT and do NOT use impersonation. When
	// impersonating, we must launch a new auth server to actually perform it
	// there.
	//
	// If we can't reuse the existing LUCI_CONTEXT, launch a new one (deriving
	// a new context.Context with it).
	canInherit := opts.Method == auth.LUCIContextMethod && opts.ActAsServiceAccount == ""
	if !canInherit {
		var la *lucictx.LocalAuth
		if ac.luciSrv, la, err = launchSrv(ac.ctx, opts, ac.authenticator, ac.ID); err != nil {
			return nil, errors.Annotate(err, "failed to launch local auth server for %q account", ac.ID).Err()
		}
		ac.ctx = lucictx.SetLocalAuth(ac.ctx, la) // switch to new LUCI_CONTEXT
	}

	// Drop the new LUCI_CONTEXT file to the disk. This is noop when reusing
	// an existing one.
	if ac.exported, err = lucictx.ExportInto(ac.ctx, tempDir); err != nil {
		return nil, errors.Annotate(err, "failed to export LUCI_CONTEXT for %q account", ac.ID).Err()
	}

	// Now setup various credential helpers (they all mutate 'ac' and return
	// annotated errors).
	if ac.EnableGitAuth {
		if err := ac.setupGitAuth(tempDir); err != nil {
			return nil, err
		}
	}
	if ac.EnableDockerAuth {
		if err := ac.setupDockerAuth(tempDir); err != nil {
			return nil, err
		}
	}
	if ac.EnableDevShell && !ac.anonymous {
		if err := ac.setupDevShellAuth(tempDir); err != nil {
			return nil, err
		}
	}
	if ac.EnableFirebaseAuth && !ac.anonymous {
		if err := ac.setupFirebaseAuth(); err != nil {
			return nil, err
		}
	}

	return
}

// Close stops this context, cleaning up after it.
//
// The context is not usable after this. Logs errors inside (there's nothing
// caller can do about them anyway).
func (ac *Context) Close() {
	// Stop all the servers in parallel.
	wg := sync.WaitGroup{}
	stop := func(what string, srv interface{ Stop(context.Context) error }) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := srv.Stop(ac.ctx); err != nil {
				logging.Errorf(ac.ctx, "Failed to stop %s server for %q account: %s", what, ac.ID, err)
			}
		}()
	}
	// Note: can't move != nil check into stop(...) because 'srv' becomes
	// a "typed nil interface", which is not nil itself.
	if ac.luciSrv != nil {
		stop("local auth", ac.luciSrv)
	}
	if ac.gsutilSrv != nil {
		stop("gsutil shim", ac.gsutilSrv)
	}
	if ac.devShellSrv != nil {
		stop("devshell", ac.devShellSrv)
	}
	if ac.firebaseSrv != nil {
		stop("firebase shim", ac.firebaseSrv)
	}
	wg.Wait()

	// Cleanup exported LUCI_CONTEXT.
	if ac.exported != nil {
		if err := ac.exported.Close(); err != nil {
			logging.Errorf(ac.ctx, "Failed to delete exported LUCI_CONTEXT for %q account - %s", ac.ID, err)
		}
	}

	// Cleanup the rest of the garbage.
	cleanup := func(what, where string) {
		if where != "" {
			if err := os.RemoveAll(where); err != nil {
				logging.Errorf(ac.ctx, "Failed to clean up %s for %q account at [%s]: %s", what, ac.ID, where, err)
			}
		}
	}
	cleanup("git HOME", ac.gitHome)
	cleanup("gsutil state", ac.gsutilState)
	cleanup("docker configs", ac.dockerConfig)
	cleanup("docker temp dir", ac.dockerTmpDir)
	cleanup("created temp dir", ac.tmpDir)

	// And finally reset the state as if nothing happened.
	ac.ctx = nil
	ac.exported = nil
	ac.tmpDir = ""
	ac.authenticator = nil
	ac.anonymous = false
	ac.email = ""
	ac.luciSrv = nil
	ac.gitHome = ""
	ac.dockerConfig = ""
	ac.dockerTmpDir = ""
	ac.gsutilSrv = nil
	ac.gsutilState = ""
	ac.gsutilBoto = ""
	ac.devShellSrv = nil
	ac.devShellAddr = nil
	ac.firebaseSrv = nil
	ac.firebaseTokenURL = ""
}

// Authenticator returns an authenticator used by this context.
//
// It is the one constructed from Options. It is safe to use it directly.
func (ac *Context) Authenticator() *auth.Authenticator {
	return ac.authenticator
}

// ExportIntoEnv exports details of this context into the environment, so it can
// be inherited by subprocesses that supports it.
//
// Returns a modified copy of 'env'.
func (ac *Context) ExportIntoEnv(env environ.Env) environ.Env {
	env = env.Clone()
	ac.exported.SetInEnviron(env)

	if ac.EnableGitAuth {
		env.Set("GIT_TERMINAL_PROMPT", "0")           // no interactive prompts
		env.Set("GIT_CONFIG_NOSYSTEM", "1")           // no $(prefix)/etc/gitconfig
		env.Set("INFRA_GIT_WRAPPER_HOME", ac.gitHome) // tell gitwrapper about the new HOME
	}

	if ac.EnableDockerAuth {
		env.Set("DOCKER_CONFIG", ac.dockerConfig)
		env.Set("DOCKER_TMPDIR", ac.dockerTmpDir)
	}

	if ac.EnableDevShell {
		env.Remove("BOTO_PATH") // avoid picking up bot-local configs, if any
		if ac.anonymous {
			// Make sure gsutil is not picking up any stale .boto configs randomly
			// laying around on the bot. Setting BOTO_CONFIG to empty dir disables
			// default ~/.boto.
			env.Set("BOTO_CONFIG", "")
		} else {
			// Point gsutil to use our auth shim server and export devshell port.
			env.Set("BOTO_CONFIG", ac.gsutilBoto)
			if ac.devShellAddr != nil {
				env.Set(devshell.EnvKey, fmt.Sprintf("%d", ac.devShellAddr.Port))
			} else {
				// See https://crbug.com/788058#c14.
				logging.Warningf(ac.ctx, "Disabling devshell auth for account %q", ac.ID)
			}
		}
	}

	if ac.EnableFirebaseAuth && !ac.anonymous {
		// Point firebase to the generated token.
		env.Set("FIREBASE_TOKEN_URL", ac.firebaseTokenURL)
	}

	return env
}

// Report logs the service account email used by this auth context.
func (ac *Context) Report() {
	account := ac.email
	if ac.anonymous {
		account = "anonymous"
	}
	logging.Infof(ac.ctx,
		"%q account is %s (git_auth: %v, devshell: %v, docker:%v, firebase: %v)",
		ac.ID, account, ac.EnableGitAuth, ac.EnableDevShell, ac.EnableDockerAuth, ac.EnableFirebaseAuth)
}

////////////////////////////////////////////////////////////////////////////////

// launchSrv launches new localauth.Server that serves LUCI_CONTEXT protocol.
//
// Returns the server itself (so it can be stopped) and also LocalAuth section
// that can be put into LUCI_CONTEXT to make subprocesses use the server.
func launchSrv(ctx context.Context, opts auth.Options, athn *auth.Authenticator, accID string) (srv *localauth.Server, la *lucictx.LocalAuth, err error) {
	// Two cases here:
	//  1) We are using options that specify service account private key or
	//     IAM-based authenticator (with IAM refresh token just initialized
	//     above). In this case we can mint tokens for any requested combination
	//     of scopes and can use NewFlexibleGenerator.
	//  2) We are using options that specify some externally configured
	//     authenticator (like GCE metadata server, or a refresh token). In this
	//     case we have to use this specific authenticator for generating tokens.
	var gen localauth.TokenGenerator
	if auth.AllowsArbitraryScopes(ctx, opts) {
		logging.Debugf(ctx, "Using flexible token generator: %s (acting as %q)", opts.Method, opts.ActAsServiceAccount)
		gen, err = localauth.NewFlexibleGenerator(ctx, opts)
	} else {
		// An authenticator preconfigured with given list of scopes.
		logging.Debugf(ctx, "Using rigid token generator: %s (scopes %s)", opts.Method, opts.Scopes)
		gen, err = localauth.NewRigidGenerator(ctx, athn)
	}
	if err != nil {
		return
	}

	// We currently always setup a context with one account (which is also
	// default). It means if we override some existing LUCI_CONTEXT, all
	// non-default accounts there are "forgotten".
	srv = &localauth.Server{
		TokenGenerators: map[string]localauth.TokenGenerator{
			accID: gen,
		},
		DefaultAccountID: accID,
	}
	la, err = srv.Start(ctx)
	return
}

func (ac *Context) setupGitAuth(tempDir string) error {
	ac.gitHome = filepath.Join(tempDir, "git-home-"+ac.ID)
	if err := os.Mkdir(ac.gitHome, 0700); err != nil {
		return errors.Annotate(err, "failed to create git HOME for %q account at %s", ac.ID, ac.gitHome).Err()
	}
	if err := ac.writeGitConfig(); err != nil {
		return errors.Annotate(err, "failed to setup .gitconfig for %q account", ac.ID).Err()
	}
	return nil
}

func (ac *Context) writeGitConfig() error {
	var cfg gitConfig
	if !ac.anonymous {
		cfg = gitConfig{
			IsWindows:           runtime.GOOS == "windows",
			UserEmail:           ac.email,
			UserName:            strings.Split(ac.email, "@")[0],
			UseCredentialHelper: true,
			KnownGerritHosts:    ac.KnownGerritHosts,
		}
	} else {
		cfg = gitConfig{
			IsWindows:           runtime.GOOS == "windows",
			UserEmail:           "anonymous@example.com", // otherwise git doesn't work
			UserName:            "anonymous",
			UseCredentialHelper: false, // fetch will be anonymous, push will fail
			KnownGerritHosts:    nil,   // don't force non-anonymous fetch for public hosts
		}
	}
	if shouldEnableGitProtocolV2() {
		cfg.GitProtocolVersion = 2
	}
	return cfg.Write(filepath.Join(ac.gitHome, ".gitconfig"))
}

func (ac *Context) setupDockerAuth(tempDir string) error {
	ac.dockerConfig = filepath.Join(tempDir, "docker-cfg-"+ac.ID)
	if err := os.Mkdir(ac.dockerConfig, 0700); err != nil {
		return errors.Annotate(err, "failed to create Docker configuration directory for %q account at %s", ac.ID, ac.dockerConfig).Err()
	}
	if err := ac.writeDockerConfig(); err != nil {
		return errors.Annotate(err, "failed to create config.json for %q account", ac.ID).Err()
	}
	ac.dockerTmpDir = filepath.Join(tempDir, "docker-tmp-"+ac.ID)
	if err := os.Mkdir(ac.dockerTmpDir, 0700); err != nil {
		return errors.Annotate(err, "failed to create Docker temporary directory for %q account at %s", ac.ID, ac.dockerTmpDir).Err()
	}
	return nil
}

func (ac *Context) writeDockerConfig() error {
	f, err := os.Create(filepath.Join(ac.dockerConfig, "config.json"))
	if err != nil {
		return err
	}
	defer f.Close()
	config := map[string]string{
		"credsStore": "luci",
	}
	if err := json.NewEncoder(f).Encode(&config); err != nil {
		return errors.Annotate(err, "cannot encode configuration").Err()
	}
	return f.Close()
}

func (ac *Context) setupDevShellAuth(tempDir string) error {
	source, err := ac.authenticator.TokenSource()
	if err != nil {
		return errors.Annotate(err, "failed to get token source for %q account", ac.ID).Err()
	}

	// The directory for .boto and gsutil credentials cache (including access
	// tokens).
	ac.gsutilState = filepath.Join(tempDir, "gsutil-"+ac.ID)
	if err := os.Mkdir(ac.gsutilState, 0700); err != nil {
		return errors.Annotate(err, "failed to create gsutil state dir for %q account at %s", ac.ID, ac.gsutilState).Err()
	}

	// Launch gsutil auth shim server. It will put a specially constructed .boto
	// into gsutilState dir (and return path to it).
	ac.gsutilSrv = &gsutil.Server{
		Source:   source,
		StateDir: ac.gsutilState,
	}
	if ac.gsutilBoto, err = ac.gsutilSrv.Start(ac.ctx); err != nil {
		return errors.Annotate(err, "failed to start gsutil auth shim server for %q account", ac.ID).Err()
	}

	// Presence of DevShell env var breaks gsutil on Windows. Luckily, we rarely
	// need to use gcloud in Windows, and gsutil (which we do use on Windows
	// extensively) is covered by gsutil auth shim server setup above.
	if runtime.GOOS != "windows" {
		ac.devShellSrv = &devshell.Server{
			Source: source,
			Email:  ac.email,
		}
		if ac.devShellAddr, err = ac.devShellSrv.Start(ac.ctx); err != nil {
			return errors.Annotate(err, "failed to start the DevShell server").Err()
		}
	}

	return nil
}

func (ac *Context) setupFirebaseAuth() error {
	source, err := ac.authenticator.TokenSource()
	if err != nil {
		return errors.Annotate(err, "failed to get token source for %q account", ac.ID).Err()
	}
	// Launch firebase auth shim server. It will provide an URL from which we'll
	// fetch an auth token.
	ac.firebaseSrv = &firebase.Server{
		Source: source,
	}
	if ac.firebaseTokenURL, err = ac.firebaseSrv.Start(ac.ctx); err != nil {
		return errors.Annotate(err, "failed to start firebase auth shim server for %q account", ac.ID).Err()
	}
	return nil
}
