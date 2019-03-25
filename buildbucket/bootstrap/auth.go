// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/integration/devshell"
	"go.chromium.org/luci/auth/integration/firebase"
	"go.chromium.org/luci/auth/integration/gsutil"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/lucictx"

	"infra/libs/infraenv"
)

// OAuthScopes defines OAuth scopes used by Kitchen itself.
//
// This is superset of all scopes we might need. It is more efficient to create
// a single token with all the scopes than make a bunch of smaller-scoped
// tokens. We trust Google APIs enough to send widely-scoped tokens to them.
//
// Note that kitchen subprocesses (git, recipes engine, etc) are still free to
// request whatever scopes they need (though LUCI_CONTEXT protocol). The scopes
// here are only for parts of Kitchen (LogDog client, BigQuery export, Devshell
// proxy, etc).
//
// See https://developers.google.com/identity/protocols/googlescopes for list of
// available scopes.
var OAuthScopes = []string{
	"https://www.googleapis.com/auth/cloud-platform",
	"https://www.googleapis.com/auth/userinfo.email",
}

// AuthContext represents some single service account to use for calls.
//
// Such context can be used by Kitchen itself or by subprocesses launched by
// Kitchen (if they are launched with environ populated via ExportIntoEnv).
//
// There are two such contexts: a system context and a recipe context.
//
// The system auth context is used for fetching recipes with git, running logdog
// and flushing metrics to BigQuery. On Swarming all these actions will use
// bot-associated account (specified in Swarming bot config), whose logical name
// (usually "system") is provided via "-luci-system-account" command-line flag.
//
// The recipe auth context is used for actually running the recipe. It is the
// context the kitchen starts with by default. On Swarming this will be the
// context associated with service account specified in the Swarming task
// definition.
//
// AuthContext is more than just a LUCI_CONTEXT with an appropriate account
// selected as default. It also implements necessary support for running third
// party tools (such as git and gsutil) with proper authentication.
type AuthContext struct {
	// ID is used in logs and file names, it is friendly name of this context.
	ID string

	// LocalAuth defines authentication related configuration that propagates to
	// the subprocesses through LUCI_CONTEXT.
	//
	// In particular it defines what logical account (e.g. "system" or "task") to
	// use by default in this context.
	LocalAuth *lucictx.LocalAuth

	// EnableGitAuth enables authentication for git subprocesses.
	//
	// Assumes 'git' binary is actually gitwrapper and that 'git-credential-luci'
	// binary is in PATH.
	EnableGitAuth bool

	// EnableDevShell enables DevShell server and gsutil auth shim.
	//
	// They are used to make gsutil and gcloud use LUCI authentication.
	//
	// On Windows only gsutil auth shim is enabled, since enabling DevShell there
	// triggers bugs in gsutil. See https://crbug.com/788058#c14.
	EnableDevShell bool

	// EnableDockerAuth enables authentication for Docker.
	//
	// Assumes 'docker-credential-luci' is in PATH.
	EnableDockerAuth bool

	// EnableFirebaseAuth enables Firebase auth shim.
	//
	// It is used to make Firebase use LUCI authentication.
	EnableFirebaseAuth bool

	// KnownGerritHosts is list of Gerrit hosts to force git authentication for.
	//
	// By default public hosts are accessed anonymously, and the anonymous access
	// has very low quota. Kitchen needs to know all such hostnames in advance to
	// be able to force authenticated access to them.
	KnownGerritHosts []string

	ctx           context.Context     // stores modified LUCI_CONTEXT
	exported      lucictx.Exported    // exported LUCI_CONTEXT on disk
	authenticator *auth.Authenticator // used by Kitchen itself
	anonymous     bool                // true if not associated with any account
	email         string              // an account email or "" for anonymous

	gsutilSrv   *gsutil.Server // gsutil auth shim server
	gsutilState string         // path to a kitchen-managed state directory
	gsutilBoto  string         // path to a generated .boto file

	firebaseSrv      *firebase.Server // firebase auth shim server
	firebaseTokenURL string           // URL to get firebase auth token from

	devShellSrv  *devshell.Server // DevShell server instance
	devShellAddr *net.TCPAddr     // address local DevShell instance is listening on

	gitHome string // custom HOME for git or "" if not using git auth

	dockerConfig string // location for Docker configuration files
	dockerTmpDir string // location for Docker temporary files
}

// Launch launches this auth context. It must be called before any other method.
//
// Callers shouldn't modify AuthContext fields after Launch is called.
func (ac *AuthContext) Launch(ctx context.Context, tempDir string) (err error) {
	defer func() {
		if err != nil {
			ac.Close()
		}
	}()

	ac.ctx = lucictx.SetLocalAuth(ctx, ac.LocalAuth)
	if ac.exported, err = lucictx.Export(ac.ctx); err != nil {
		return errors.Annotate(err, "failed to export LUCI_CONTEXT for %q account", ac.ID).Err()
	}

	// Construct authentication with default set of scopes to be used through out
	// Kitchen.
	authOpts := infraenv.DefaultAuthOptions()
	authOpts.Scopes = append([]string(nil), OAuthScopes...)
	if ac.EnableFirebaseAuth {
		authOpts.Scopes = append(authOpts.Scopes, "https://www.googleapis.com/auth/firebase")
	}
	ac.authenticator = auth.NewAuthenticator(ac.ctx, auth.SilentLogin, authOpts)

	// Figure out what email is associated with this account (if any).
	ac.email, err = ac.authenticator.GetEmail()
	switch {
	case err == auth.ErrLoginRequired:
		// This context is not associated with any account. This happens when
		// running Swarming tasks without service account specified.
		ac.anonymous = true
	case err != nil:
		return errors.Annotate(err, "failed to get email of %q account", ac.ID).Err()
	}

	if ac.EnableGitAuth {
		// Create new HOME for git and populate it with .gitconfig.
		ac.gitHome = filepath.Join(tempDir, "git_home_"+ac.ID)
		if err := os.Mkdir(ac.gitHome, 0700); err != nil {
			return errors.Annotate(err, "failed to create git HOME for %q account at %s", ac.ID, ac.gitHome).Err()
		}
		if err := ac.writeGitConfig(); err != nil {
			return errors.Annotate(err, "failed to setup .gitconfig for %q account", ac.ID).Err()
		}
	}

	if ac.EnableDockerAuth {
		// Create directory for Docker configuration files and write config.json there.
		ac.dockerConfig = filepath.Join(tempDir, "docker_cfg_"+ac.ID)
		if err := os.Mkdir(ac.dockerConfig, 0700); err != nil {
			return errors.Annotate(err, "failed to create Docker configuration directory for %q account at %s", ac.ID, ac.dockerConfig).Err()
		}
		if err := ac.writeDockerConfig(); err != nil {
			return errors.Annotate(err, "failed to create config.json for %q account", ac.ID).Err()
		}
		ac.dockerTmpDir = filepath.Join(tempDir, "docker_tmp_"+ac.ID)
		if err := os.Mkdir(ac.dockerTmpDir, 0700); err != nil {
			return errors.Annotate(err, "failed to create Docker temporary directory for %q account at %s", ac.ID, ac.dockerTmpDir).Err()
		}
	}

	if ac.EnableFirebaseAuth && !ac.anonymous {
		source, err := ac.authenticator.TokenSource()
		if err != nil {
			return errors.Annotate(err, "failed to get token source for %q account", ac.ID).Err()
		}
		// Launch firebase auth shim server. It will provide a URL from which we'll fetch an auth token.
		ac.firebaseSrv = &firebase.Server{
			Source: source,
		}
		if ac.firebaseTokenURL, err = ac.firebaseSrv.Start(ctx); err != nil {
			return errors.Annotate(err, "failed to start firebase auth shim server for %q account", ac.ID).Err()
		}
	}

	if ac.EnableDevShell && !ac.anonymous {
		source, err := ac.authenticator.TokenSource()
		if err != nil {
			return errors.Annotate(err, "failed to get token source for %q account", ac.ID).Err()
		}

		// The directory for .boto and gsutil credentials cache (including access
		// tokens).
		ac.gsutilState = filepath.Join(tempDir, "gsutil_"+ac.ID)
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
	}

	return nil
}

// Close stops this context, cleaning up after it.
//
// The context is not usable after this. Logs errors inside (there's nothing
// caller can do about them anyway).
func (ac *AuthContext) Close() {
	if ac.gitHome != "" {
		if err := os.RemoveAll(ac.gitHome); err != nil {
			logging.Errorf(ac.ctx, "Failed to clean up git HOME for %q account at [%s]: %s", ac.ID, ac.gitHome, err)
		}
	}

	if ac.gsutilSrv != nil {
		if err := ac.gsutilSrv.Stop(ac.ctx); err != nil {
			logging.Errorf(ac.ctx, "Failed to stop gsutil shim server for %q account: %s", ac.ID, err)
		}
	}

	if ac.gsutilState != "" {
		if err := os.RemoveAll(ac.gsutilState); err != nil {
			logging.Errorf(ac.ctx, "Failed to clean up gsutil state for %q account at [%s]: %s", ac.ID, ac.gsutilState, err)
		}
	}

	if ac.firebaseSrv != nil {
		if err := ac.firebaseSrv.Stop(ac.ctx); err != nil {
			logging.Errorf(ac.ctx, "Failed to stop firebase shim server for %q account: %s", ac.ID, err)
		}
	}

	if ac.devShellSrv != nil {
		if err := ac.devShellSrv.Stop(ac.ctx); err != nil {
			logging.Errorf(ac.ctx, "Failed to stop DevShell server for %q account: %s", ac.ID, err)
		}
	}

	if ac.dockerConfig != "" {
		if err := os.RemoveAll(ac.dockerConfig); err != nil {
			logging.Errorf(ac.ctx, "Failed to clean up Docker configuration directory for %q account at [%s]: %s", ac.ID, ac.dockerConfig, err)
		}
	}

	if ac.dockerTmpDir != "" {
		if err := os.RemoveAll(ac.dockerTmpDir); err != nil {
			logging.Errorf(ac.ctx, "Failed to clean up Docker temporary directory for %q account at [%s]: %s", ac.ID, ac.dockerTmpDir, err)
		}
	}

	if ac.exported != nil {
		if err := ac.exported.Close(); err != nil {
			logging.Errorf(ac.ctx, "Failed to delete exported LUCI_CONTEXT for %q account - %s", ac.ID, err)
		}
	}

	ac.ctx = nil
	ac.exported = nil
	ac.authenticator = nil
	ac.anonymous = false
	ac.email = ""
	ac.gsutilSrv = nil
	ac.gsutilState = ""
	ac.gsutilBoto = ""
	ac.firebaseSrv = nil
	ac.firebaseTokenURL = ""
	ac.devShellSrv = nil
	ac.devShellAddr = nil
	ac.gitHome = ""
	ac.dockerConfig = ""
	ac.dockerTmpDir = ""
}

// Authenticator returns an authenticator that can be used by Kitchen itself.
//
// It uses the default set of scopes, see OAuthScopes.
func (ac *AuthContext) Authenticator() *auth.Authenticator {
	return ac.authenticator
}

// ExportIntoEnv exports details of this context into the environment, so it can
// be inherited by subprocesses that supports it.
//
// Returns a modified copy of 'env'.
func (ac *AuthContext) ExportIntoEnv(env environ.Env) environ.Env {
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

	if ac.EnableFirebaseAuth && !ac.anonymous {
		// Point firebase to the generated token.
		env.Set("FIREBASE_TOKEN_URL", ac.firebaseTokenURL)
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

	return env
}

// ReportServiceAccount logs service account email used by this auth context.
func (ac *AuthContext) ReportServiceAccount() {
	account := ac.email
	if ac.anonymous {
		account = "anonymous"
	}
	logging.Infof(ac.ctx, "%q account is %s (git_auth: %v, devshell: %v, docker:%v, firebase: %v)",
		ac.ID, account, ac.EnableGitAuth, ac.EnableDevShell, ac.EnableDockerAuth, ac.EnableFirebaseAuth)
}

////

func (ac *AuthContext) writeGitConfig() error {
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

func (ac *AuthContext) writeDockerConfig() error {
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
