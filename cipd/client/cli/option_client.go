// Copyright 2026 The LUCI Authors.
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

package cli

import (
	"context"
	"flag"
	"fmt"
	"net/http"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/environ"

	"go.chromium.org/luci/cipd/client/cipd"
	"go.chromium.org/luci/cipd/client/cipd/ensure"
	"go.chromium.org/luci/cipd/common/cipderr"
)

////////////////////////////////////////////////////////////////////////////////
// clientOptions mixin.

type rootDirFlag bool

const (
	withRootDir    rootDirFlag = true
	withoutRootDir rootDirFlag = false
)

type maxThreadsFlag bool

const (
	withMaxThreads    maxThreadsFlag = true
	withoutMaxThreads maxThreadsFlag = false
)

// clientOptions defines command line arguments related to CIPD client creation.
// Subcommands that need a CIPD client embed it.
type clientOptions struct {
	hardcoded Parameters // whatever was passed to registerFlags(...)

	cliServiceURL string
	cacheDir      string
	maxThreads    maxThreadsOption
	rootDir       string              // used only if registerFlags got withRootDir arg
	versions      ensure.VersionsFile // mutated by loadEnsureFile

	authFlags authcli.Flags
}

func (opts *clientOptions) resolvedServiceURL(ctx context.Context, ensureFileURL string) string {
	if ensureFileURL != "" {
		return ensureFileURL
	}
	if opts.cliServiceURL != "" {
		return opts.cliServiceURL
	}
	if v := environ.FromCtx(ctx).Get(cipd.EnvCIPDServiceURL); v != "" {
		return v
	}
	return opts.hardcoded.ServiceURL
}

func (opts *clientOptions) registerFlags(f *flag.FlagSet, params Parameters, rootDir rootDirFlag, maxThreads maxThreadsFlag) {
	opts.hardcoded = params

	f.StringVar(&opts.cliServiceURL, "service-url", "",
		fmt.Sprintf(`Backend URL. If provided via an "ensure" file, the URL in the file takes precedence. `+
			`(default %s)`, params.ServiceURL))
	f.StringVar(&opts.cacheDir, "cache-dir", "",
		fmt.Sprintf("Directory for the shared cache (can also be set by %s env var).", cipd.EnvCacheDir))

	if rootDir {
		f.StringVar(&opts.rootDir, "root", "<path>", "Path to an installation site root directory.")
	}
	if maxThreads {
		opts.maxThreads.registerFlags(f)
	}

	opts.authFlags.Register(f, params.DefaultAuthOptions)
	opts.authFlags.RegisterCredentialHelperFlags(f)
	opts.authFlags.RegisterADCFlags(f)
}

func (opts *clientOptions) toCIPDClientOpts(ctx context.Context, ensureFileURL string) (cipd.ClientOptions, error) {
	authOpts, err := opts.authFlags.Options()
	if err != nil {
		return cipd.ClientOptions{}, cipderr.BadArgument.Apply(errors.Fmt("bad auth options: %w", err))
	}

	realOpts := cipd.ClientOptions{
		Root:              opts.rootDir,
		CacheDir:          opts.cacheDir,
		Versions:          opts.versions,
		MaxThreads:        opts.maxThreads.maxThreads,
		AnonymousClient:   http.DefaultClient,
		PluginsContext:    ctx,
		LoginInstructions: "run `cipd auth-login` to login or relogin",
	}
	if err := realOpts.LoadFromEnv(ctx); err != nil {
		return cipd.ClientOptions{}, err
	}
	realOpts.ServiceURL = opts.resolvedServiceURL(ctx, ensureFileURL)

	// When using a proxy, the proxy does authenticated requests to the backend
	// and the client just hits it anonymously (see ClientOptions.ProxyURL). Skip
	// loading credentials.
	if realOpts.ProxyURL != "" {
		logging.Debugf(ctx, "Using %s=%s", cipd.EnvCIPDProxyURL, realOpts.ProxyURL)
	} else {
		realOpts.AuthenticatedClient, err = auth.NewAuthenticator(ctx, auth.OptionalLogin, authOpts).Client()
		if err != nil {
			return cipd.ClientOptions{}, cipderr.Auth.Apply(errors.Fmt("initializing auth client: %w", err))
		}
	}

	return realOpts, nil
}

func (opts *clientOptions) makeCIPDClient(ctx context.Context, ensureFileURL string) (cipd.Client, error) {
	cipdOpts, err := opts.toCIPDClientOpts(ctx, ensureFileURL)
	if err != nil {
		return nil, err
	}
	return cipd.NewClient(cipdOpts)
}
