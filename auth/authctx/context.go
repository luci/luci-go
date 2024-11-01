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
	"go.chromium.org/luci/auth/integration/gcemeta"
	"go.chromium.org/luci/auth/integration/gsutil"
	"go.chromium.org/luci/auth/integration/localauth"
)

// Context knows how to prepare an environment with ambient authentication for
// various tools: LUCI, gsutil, Docker, Git, Firebase.
//
// 'Launch' launches a bunch of local HTTP servers and writes a bunch of
// configuration files that point to these servers. 'Export' then exposes
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
	// modified by 'Export' will see a functional LUCI_CONTEXT.
	//
	// When reusing an existing LUCI_CONTEXT, subprocesses inherit all OAuth
	// scopes permissible there.
	Options auth.Options

	// ExposeSystemAccount indicates if this authentication context should also
	// expose non-default "system" logical LUCI account (using the same
	// credentials as the default account).
	//
	// This is an advanced feature used to emulate Swarming environment.
	ExposeSystemAccount bool

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
	//
	// TODO(vadimsh): Delete this method if EnableGCEEmulation works everywhere.
	EnableDevShell bool

	// EnableGCEEmulation enables emulation of GCE instance environment.
	//
	// Overrides EnableDevShell if used. Will likely completely replace
	// EnableDevShell in the near future.
	//
	// It does multiple things by setting environment variables and writing config
	// files:
	//   * Creates new empty CLOUDSDK_CONFIG directory, to make sure we don't
	//     reuse existing gcloud cache.
	//   * Creates new BOTO_CONFIG, telling gsutil to use new empty state dir.
	//   * Launches a local server that imitates GCE metadata server.
	//   * Tells gcloud, gsutil and various Go and Python libraries to use this
	//     server by setting env vars like GCE_METADATA_HOST (and a bunch more).
	//
	// This tricks gcloud, gsutil and various Go and Python libraries that use
	// Application Default Credentials into believing they run on GCE so that
	// they request OAuth2 tokens via GCE metadata server (which is implemented by
	// us).
	//
	// This is not a foolproof way: nothing prevents clients from ignoring env
	// vars and hitting metadata.google.internal directly. But most clients
	// respect env vars we set.
	EnableGCEEmulation bool

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

	localAuth     *lucictx.LocalAuth  // non-nil when running localauth.Server
	tmpDir        string              // non empty if we created a new temp dir
	authenticator *auth.Authenticator // used by in-process helpers
	anonymous     bool                // true if not associated with any account
	email         string              // an account email or "" for anonymous
	luciSrv       *localauth.Server   // non-nil if we launched a LUCI_CONTEXT subcontext

	gitHome string // custom HOME for git or "" if not using git auth

	dockerConfig string // location for Docker configuration files
	dockerTmpDir string // location for Docker temporary files

	gsutilSrv *gsutil.Server // gsutil auth shim server

	// Note: these fields are used in both EnableGCEEmulation and EnableDevShell
	// modes.
	gsutilState string // path to a context-managed state directory
	gsutilBoto  string // path to a generated .boto file

	devShellSrv  *devshell.Server // DevShell server instance
	devShellAddr *net.TCPAddr     // address local DevShell instance is listening on

	gcemetaSrv    *gcemeta.Server // fake GCE metadata server
	gcemetaAddr   string          // "host:port" address of the fake metadata server
	gcloudConfDir string          // directory with gcloud configs

	firebaseSrv      *firebase.Server // firebase auth shim server
	firebaseTokenURL string           // URL to get firebase auth token from
}

// Launch launches this auth context. It must be called before any other method.
//
// It launches various local server and prepares various configs, by putting
// them into tempDir which may be "" to use some new ioutil.TempDir.
//
// The given context.Context is used for logging and to pick up the initial
// ambient authentication (per auth.NewAuthenticator contract, see its docs).
//
// To run a subprocess within this new auth context use 'Export' to modify
// an environ for a new process.
func (ac *Context) Launch(ctx context.Context, tempDir string) (err error) {
	// EnableGCEEmulation provides a superset of EnableDevShell features. No need
	// to have both enabled at the same time (they also conflict with each other).
	if ac.EnableGCEEmulation {
		ac.EnableDevShell = false
	}

	defer func() {
		if err != nil {
			ac.Close(ctx)
		}
	}()

	if tempDir == "" {
		ac.tmpDir, err = ioutil.TempDir("", "luci")
		if err != nil {
			return errors.Annotate(err, "failed to create a temp directory").Err()
		}
		tempDir = ac.tmpDir
	}

	// Make a generator that will be used to generate tokens for subprocesses
	// that request them via LUCI_CONTEXT protocol or via GCE metadata emulation.
	tokens := auth.NewTokenGenerator(ctx, ac.Options)
	opts := tokens.Options() // normalized, with opts.Method populated

	// Construct the OAuth2 authenticator to be used directly by the helpers
	// hosted in the current process (devshell, gsutil, firebase) using scopes
	// passed via ac.Options. Out-of-process helpers (git, docker) will use
	// `tokens` via the LUCI_CONTEXT protocol or GCE metadata server emulation.
	ac.authenticator, err = tokens.Authenticator(opts.Scopes, "")
	if err != nil {
		return errors.Annotate(err, "failed to construct authenticator for %q account", ac.ID).Err()
	}

	// Figure out what email is associated with this account (if any).
	ac.email, err = ac.authenticator.GetEmail()
	switch {
	case err == auth.ErrLoginRequired:
		// This context is not associated with any account. This happens when
		// running Swarming tasks without service account specified or running
		// locally without doing 'luci-auth login' first.
		ac.anonymous = true
	case err != nil:
		return errors.Annotate(err, "failed to get email of %q account", ac.ID).Err()
	}

	// Check whether we are allowed to inherit the existing LUCI_CONTEXT. We do it
	// if 'opts' indicate to use LUCI_CONTEXT and do NOT use impersonation. When
	// impersonating, we must launch a new auth server to actually perform it
	// there.
	//
	// If we can't reuse the existing LUCI_CONTEXT, launch a new one (deriving
	// a new context.Context with it).
	//
	// If there's no auth credentials at all, do not launch any LUCI_CONTEXT (it
	// is impossible without credentials). Subprocesses will discover lack of
	// ambient credentials on their own and fail (or proceed) appropriately.
	canInherit := opts.Method == auth.LUCIContextMethod && opts.ActAsServiceAccount == "" && !ac.ExposeSystemAccount
	if !canInherit && !ac.anonymous {
		if ac.luciSrv, ac.localAuth, err = launchSrv(ctx, tokens, ac.ID, ac.ExposeSystemAccount); err != nil {
			return errors.Annotate(err, "failed to launch local auth server for %q account", ac.ID).Err()
		}
	}

	// Now setup various credential helpers (they all mutate 'ac' and return
	// annotated errors).
	if ac.EnableGitAuth {
		if err := ac.setupGitAuth(tempDir); err != nil {
			return err
		}
	}
	if ac.EnableDockerAuth {
		if err := ac.setupDockerAuth(tempDir); err != nil {
			return err
		}
	}
	if ac.EnableDevShell && !ac.anonymous {
		if err := ac.setupDevShellAuth(ctx, tempDir); err != nil {
			return err
		}
	}
	if ac.EnableGCEEmulation {
		if err := ac.setupGCEEmulationAuth(ctx, tokens, tempDir); err != nil {
			return err
		}
	}
	if ac.EnableFirebaseAuth && !ac.anonymous {
		if err := ac.setupFirebaseAuth(ctx); err != nil {
			return err
		}
	}

	return nil
}

// Close stops this context, cleaning up after it.
//
// The given context.Context is used for deadlines and for logging.
//
// The auth context is not usable after this call. Logs errors inside (there's
// nothing caller can do about them anyway).
func (ac *Context) Close(ctx context.Context) {
	// Stop all the servers in parallel.
	wg := sync.WaitGroup{}
	stop := func(what string, srv interface{ Stop(context.Context) error }) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := srv.Stop(ctx); err != nil {
				logging.Errorf(ctx, "Failed to stop %s server for %q account: %s", what, ac.ID, err)
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
	if ac.gcemetaSrv != nil {
		stop("fake GCE metadata server", ac.gcemetaSrv)
	}
	if ac.firebaseSrv != nil {
		stop("firebase shim", ac.firebaseSrv)
	}
	wg.Wait()

	// Cleanup the rest of the garbage.
	cleanup := func(what, where string) {
		if where != "" {
			if err := os.RemoveAll(where); err != nil {
				logging.Errorf(ctx, "Failed to clean up %s for %q account at [%s]: %s", what, ac.ID, where, err)
			}
		}
	}
	cleanup("git HOME", ac.gitHome)
	cleanup("gsutil state", ac.gsutilState)
	cleanup("gcloud config dir", ac.gcloudConfDir)
	cleanup("docker configs", ac.dockerConfig)
	cleanup("docker temp dir", ac.dockerTmpDir)
	cleanup("created temp dir", ac.tmpDir)

	// And finally reset the state as if nothing happened.
	ac.localAuth = nil
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
	ac.gcemetaSrv = nil
	ac.gcemetaAddr = ""
	ac.gcloudConfDir = ""
	ac.firebaseSrv = nil
	ac.firebaseTokenURL = ""
}

// Authenticator returns an authenticator used by this context.
//
// It is the one constructed from Options. It is safe to use it directly.
func (ac *Context) Authenticator() *auth.Authenticator {
	return ac.authenticator
}

// Export exports details of this context into the environment, so it can
// be inherited by subprocesses that support it.
//
// It does two inter-dependent things:
//  1. Updates LUCI_CONTEXT in 'ctx' so that LUCI tools can use the local
//     token server.
//  2. Mutates 'env' so that various third party tools can also use local
//     tokens.
//
// To successfully launch a subprocess, LUCI_CONTEXT in returned context.Context
// *must* be exported into 'env' (e.g. via lucictx.Export(...) followed by
// SetInEnviron).
func (ac *Context) Export(ctx context.Context, env environ.Env) context.Context {
	// Mutate LUCI_CONTEXT to use localauth.Server{...} launched by us (if any).
	ctx = ac.SetLocalAuth(ctx)

	if ac.EnableGitAuth {
		env.Set("GIT_TERMINAL_PROMPT", "0")           // no interactive prompts
		env.Set("INFRA_GIT_WRAPPER_HOME", ac.gitHome) // tell gitwrapper about the new HOME
	}

	if ac.EnableDockerAuth {
		env.Set("DOCKER_CONFIG", ac.dockerConfig)
		env.Set("DOCKER_TMPDIR", ac.dockerTmpDir)
	}

	if ac.EnableDevShell && !ac.anonymous {
		if ac.devShellAddr != nil {
			env.Set(devshell.EnvKey, fmt.Sprintf("%d", ac.devShellAddr.Port))
		} else {
			// See https://crbug.com/788058#c14.
			logging.Warningf(ctx, "Disabling devshell auth for account %q", ac.ID)
		}
	}

	if ac.EnableGCEEmulation {
		env.Set("CLOUDSDK_CONFIG", ac.gcloudConfDir)
		if !ac.anonymous {
			// Used by google.auth.compute_engine Python library to grab tokens.
			env.Set("GCE_METADATA_ROOT", ac.gcemetaAddr)
			// Used by google.auth.compute_engine Python library to "ping" metadata srv.
			env.Set("GCE_METADATA_IP", ac.gcemetaAddr)
			// Used by cloud.google.com/go/compute/metadata Go library.
			env.Set("GCE_METADATA_HOST", ac.gcemetaAddr)
		}
	}

	// Prepare .boto configs if faking Cloud in some way. Do it even if running
	// anonymously, since in this case we want to switch gsutil to run in
	// anonymous mode as well (by forbidding it to use default ~/.boto that may
	// have some credential in it).
	if ac.EnableDevShell || ac.EnableGCEEmulation {
		// Note: gsutilBoto may be empty here if running anonymously in DevShell
		// mode. This is fine, it tells gsutil not to use default ~/.boto.
		env.Set("BOTO_CONFIG", ac.gsutilBoto)
		env.Remove("BOTO_PATH")
	}

	if ac.EnableFirebaseAuth && !ac.anonymous {
		// This env var is supposed to contain a refresh token. Its presence
		// switches Firebase into "CI mode" where it doesn't try to grab credentials
		// from disk or via gcloud. The actual value doesn't matter, since we
		// replace the endpoint that consumes this token below.
		env.Set("FIREBASE_TOKEN", "ignored-non-empty-value")
		// Instruct Firebase to use the local server for "refreshing" the token.
		// Usually this is "https://www.googleapis.com" and it takes a refresh token
		// and returns an access token. We replace it with a local version that
		// just returns task account access tokens.
		env.Set("FIREBASE_TOKEN_URL", ac.firebaseTokenURL)
	}

	return ctx
}

// SetLocalAuth updates `local_auth` section of LUCI_CONTEXT.
//
// Note that this would allow LUCI libraries to use this auth context, but
// other software (gsutil, gcloud, firebase etc) will not see it. They need
// various environment variables to be exported first. Use Export for that.
func (ac *Context) SetLocalAuth(ctx context.Context) context.Context {
	if ac.localAuth != nil {
		return lucictx.SetLocalAuth(ctx, ac.localAuth)
	}
	return ctx
}

// Report logs the service account email used by this auth context.
func (ac *Context) Report(ctx context.Context) {
	account := ac.email
	if ac.anonymous {
		account = "anonymous"
	}
	logging.Infof(ctx,
		"%q account is %s (git_auth: %v, devshell: %v, emulate_gce:%v, docker:%v, firebase: %v)",
		ac.ID, account, ac.EnableGitAuth, ac.EnableDevShell, ac.EnableGCEEmulation,
		ac.EnableDockerAuth, ac.EnableFirebaseAuth)
}

////////////////////////////////////////////////////////////////////////////////

// launchSrv launches new localauth.Server that serves LUCI_CONTEXT protocol.
//
// Returns the server itself (so it can be stopped) and also LocalAuth section
// that can be put into LUCI_CONTEXT to make subprocesses use the server.
func launchSrv(ctx context.Context, tokens *auth.TokenGenerator, accID string, exposeSystemAccount bool) (*localauth.Server, *lucictx.LocalAuth, error) {
	generators := make(map[string]localauth.TokenGenerator, 2)
	generators[accID] = tokens
	if exposeSystemAccount {
		generators["system"] = tokens
	}
	srv := &localauth.Server{
		TokenGenerators:  generators,
		DefaultAccountID: accID,
	}
	la, err := srv.Start(ctx)
	return srv, la, err
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
	config := map[string]map[string]string{
		"credHelpers": {
			"us.gcr.io":                  "luci",
			"staging-k8s.gcr.io":         "luci",
			"asia.gcr.io":                "luci",
			"gcr.io":                     "luci",
			"marketplace.gcr.io":         "luci",
			"eu.gcr.io":                  "luci",
			"us-central1-docker.pkg.dev": "luci",
		},
	}
	if err := json.NewEncoder(f).Encode(&config); err != nil {
		return errors.Annotate(err, "cannot encode configuration").Err()
	}
	return f.Close()
}

func (ac *Context) setupDevShellAuth(ctx context.Context, tempDir string) error {
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
	if ac.gsutilBoto, err = ac.gsutilSrv.Start(ctx); err != nil {
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
		if ac.devShellAddr, err = ac.devShellSrv.Start(ctx); err != nil {
			return errors.Annotate(err, "failed to start the DevShell server").Err()
		}
	}

	return nil
}

func (ac *Context) setupGCEEmulationAuth(ctx context.Context, tokens *auth.TokenGenerator, tempDir string) error {
	// Launch the fake GCE metadata server.
	botoGCEAccount := ""
	if !ac.anonymous {
		ac.gcemetaSrv = &gcemeta.Server{
			Generator:        tokens,
			Email:            ac.email,
			Scopes:           tokens.Options().Scopes,
			MinTokenLifetime: tokens.Options().MinTokenLifetime,
			InheritFromGCE:   true, // expose some real GCE info if on GCE for real
		}
		var err error
		if ac.gcemetaAddr, err = ac.gcemetaSrv.Start(ctx); err != nil {
			return errors.Annotate(err, "failed to start fake GCE metadata server for %q account", ac.ID).Err()
		}
		botoGCEAccount = "default" // switch .boto to use GCE auth
	}

	// Prepare clean gcloud config, otherwise gcloud will reuse cached "is on GCE"
	// value from ~/.config/gcloud/gce and will not bother contacting the fake GCE
	// metadata server on non-GCE machines. Additionally in anonymous mode we
	// want to avoid using any cached credentials (also stored in the default
	// ~/.config/gcloud/...).
	ac.gcloudConfDir = filepath.Join(tempDir, "gcloud-"+ac.ID)
	if err := os.Mkdir(ac.gcloudConfDir, 0700); err != nil {
		return errors.Annotate(err, "failed to create gcloud config dir for %q account at %s", ac.ID, ac.gcloudConfDir).Err()
	}

	// The directory for .boto and gsutil credentials cache. We need to replace it
	// to tell gsutil NOT to use whatever tokens it had cached in the default
	// ~/.gsutil/... state dir.
	var err error
	ac.gsutilState = filepath.Join(tempDir, "gsutil-"+ac.ID)
	ac.gsutilBoto, err = gsutil.PrepareStateDir(&gsutil.Boto{
		StateDir:          ac.gsutilState,
		GCEServiceAccount: botoGCEAccount, // may be "" in anonymous mode
	})
	return errors.Annotate(err, "failed to setup .boto for %q account", ac.ID).Err()
}

func (ac *Context) setupFirebaseAuth(ctx context.Context) error {
	source, err := ac.authenticator.TokenSource()
	if err != nil {
		return errors.Annotate(err, "failed to get token source for %q account", ac.ID).Err()
	}
	// Launch firebase auth shim server. It will provide an URL from which we'll
	// fetch an auth token.
	ac.firebaseSrv = &firebase.Server{
		Source: source,
	}
	if ac.firebaseTokenURL, err = ac.firebaseSrv.Start(ctx); err != nil {
		return errors.Annotate(err, "failed to start firebase auth shim server for %q account", ac.ID).Err()
	}
	return nil
}
