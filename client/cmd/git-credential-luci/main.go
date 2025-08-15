// Copyright 2017 The LUCI Authors.
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

// Command git-credential-luci is a Git credential helper.
//
// The protocol used for communication between Git and the credential helper
// is documented in:
//   - https://git-scm.com/docs/gitcredentials
//   - https://git-scm.com/docs/git-credential
//
// If you need ReAuth to authenticate with Gerrit, enable ReAuth by
// setting an environment variable:
//
//	LUCI_ENABLE_REAUTH=1
//
// git-credential-luci supports ancillary authentication via a plugin.
// A plugin can be configured by setting an environment variable:
//
//	GOOGLE_AUTH_WEBAUTHN_PLUGIN=luci-auth-fido2-plugin
//
// This will run the plugin binary `luci-auth-fido2-plugin` when
// needed.
//
// To enable debug logging:
//
//	LUCI_AUTH_DEBUG=1
//
// If you encounter issues with ReAuth, you may bypass the client side
// check with:
//
//	LUCI_BYPASS_REAUTH=1
//
// This will not bypass server side enforcement when it gets enabled,
// so please file a ticket so your issue can get resolved:
//
// https://issues.chromium.org/issues/new?component=1456702&template=2176581
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/gerrit"
	"go.chromium.org/luci/common/git/creds"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

var (
	flags    authcli.Flags
	lifetime time.Duration
)

func init() {
	defaults := chromeinfra.DefaultAuthOptions()
	defaults.Scopes = []string{gitiles.OAuthScope, auth.OAuthScopeEmail, auth.OAuthScopeReAuth}
	// NOTE: This OAuth client is used exclusively for Git/Gerrit authentication with this helper.
	// Do NOT try to use this client for any other purpose.
	// If you do, expect us to proactively break your use case.
	defaults.ClientID = "608762726021-o18nheno8qn3tquf7tcmmec7urudmoba.apps.googleusercontent.com"
	defaults.ClientSecret = "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"
	flags.Register(flag.CommandLine, defaults)
	flag.DurationVar(
		&lifetime, "lifetime", time.Minute,
		"Minimum token lifetime. If existing token expired and refresh token or service account is not present, returns nothing.",
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: git-credential-luci command\n")
		flag.PrintDefaults()
	}
}

// loginRequiredMsg is the message to print when reporting a login required error.
//
// In most setups, Git will fallback to reading the username and
// password, which will overwrite any preceding trailing whitespace.  Thus, we
// add a zero width space at the end to protect the message.
const loginRequiredMsg = `not running with a service account and not logged in

If you are running this in a development environment, you can log in by running:

git credential-luci login

` + "\u200b\n"

// reauthRequiredMsg is the message to print when a RAPT is required
// but missing/expired.
//
// In most setups, Git will fallback to reading the username and
// password, which will overwrite any preceding trailing whitespace.  Thus, we
// add a zero width space at the end to protect the message.
const reAuthRequiredMsg = `ReAuth is required

If you are running this in a development environment, you can fix this by running:

git credential-luci reauth

` + "\u200b\n"

func main() {
	flag.Parse()

	if len(flag.Args()) != 1 {
		fmt.Fprintln(os.Stderr, "invalid number of arguments")
		os.Exit(1)
	}

	opts, err := flags.Options()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if lifetime > 30*time.Minute {
		fmt.Fprintln(os.Stderr, "lifetime cannot exceed 30m")
		os.Exit(1)
	}

	ctx := gologger.StdConfig.Use(context.Background())
	if debugLogEnabled() {
		ctx = logging.SetLevel(ctx, logging.Debug)
	}

	switch flag.Args()[0] {
	case "get":
		handleGet(ctx, opts)
	case "logout":
		// logout is not part of the Git credential helper
		// specification, but it is provided for convenience.
		fallthrough
	case "erase":
		a := auth.NewAuthenticator(ctx, auth.SilentLogin, opts)
		if err := a.PurgeCredentialsCache(); err != nil {
			fmt.Fprintf(os.Stderr, "cannot erase cache: %v\n", err)
			os.Exit(1)
		}
	case "login":
		// This is not part of the Git credential helper
		// specification, but it is provided for convenience.

		// Create a new authenticator with interactive login.
		a := auth.NewAuthenticator(ctx, auth.InteractiveLogin, opts)
		if err := a.Login(); err != nil {
			fmt.Fprintf(os.Stderr, "login failed: %v\n", err)
			os.Exit(1)
		}
		if reAuthEnabled() {
			ra := auth.NewReAuthenticator(a)
			if err := ra.RenewRAPT(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "ReAuth failed: %v\n", err)
				os.Exit(1)
			}
		}
	case "reauth":
		// This is not part of the Git credential helper
		// specification, but is needed for ReAuth.
		a := auth.NewAuthenticator(ctx, auth.SilentLogin, opts)
		if !reAuthEnabled() {
			fmt.Fprintf(os.Stderr, "Note: ReAuth is not enabled, so git-credential-luci won't use your ReAuth credentials\n")
			fmt.Fprintf(os.Stderr, "even if you refresh them with this command.\n")
			fmt.Fprintf(os.Stderr, "Set LUCI_ENABLE_REAUTH=1 to enable\n")
		}
		ra := auth.NewReAuthenticator(a)
		if err := ra.RenewRAPT(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "ReAuth failed: %v\n", err)
			os.Exit(1)
		}
	case "info":
		// This is not part of the Git credential helper
		// specification, but it is provided for convenience.
		//
		// This prints some info about the active credentials,
		// using a format similar to the Git credential
		// protocol, but with custom keys.
		//
		// This is intended for consumption by tools to
		// report more useful errors.
		//
		// Output keys:
		//
		//  - email
		//  - has_rapt
		a := auth.NewAuthenticator(ctx, auth.SilentLogin, opts)
		email, err := a.GetEmail()
		if err != nil {
			fmt.Fprintf(os.Stderr, "cannot get email: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("email=%s\n", email)
		ra := auth.NewReAuthenticator(a)
		rapt, err := ra.GetRAPT(ctx)
		hasRAPT := err == nil && rapt != ""
		fmt.Printf("has_rapt=%v\n", hasRAPT)
	case "clear-reauth-check-cache":
		// This is not part of the Git credential helper
		// specification.
		cache := getReAuthResultCache(ctx, opts)
		switch cache := cache.(type) {
		case *gerrit.DiskResultCache:
			if err := cache.Clear(); err != nil {
				logging.Warningf(ctx, "Error clearing cache: %s", err)
			}
		default:
			logging.Debugf(ctx, "Cannot purge cache of type %T", cache)
		}
	default:
		// The specification for Git credential helper says: "If a helper
		// receives any other operation, it should silently ignore the
		// request."
	}
}

func handleGet(ctx context.Context, opts auth.Options) {
	a := auth.NewAuthenticator(ctx, auth.SilentLogin, opts)
	t, err := a.GetAccessToken(lifetime)
	if err != nil {
		if errors.Is(err, auth.ErrLoginRequired) {
			fmt.Fprint(os.Stderr, loginRequiredMsg)
		} else {
			fmt.Fprintf(os.Stderr, "cannot get access token: %v\n", err)
		}
		os.Exit(1)
	}
	if reAuthEnabled() {
		logging.Debugf(ctx, "ReAuth is enabled")
		var readInput bytes.Buffer
		attrs, err := creds.ReadAttrs(io.TeeReader(os.Stdin, &readInput))
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		logging.Debugf(ctx, "Read input %q", readInput.Bytes())
		logging.Debugf(ctx, "Got attributes %+v", attrs)

		res, err := checkReAuth(ctx, a, opts, attrs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		if res.NeedsRAPT || forceReAuth() {
			if !attrs.HasAuthtypeCapability() {
				fmt.Fprintf(os.Stderr, "Git client does not support authtype capability, so cannot continue with ReAuth\n")
				os.Exit(1)
			}
			ra := auth.NewReAuthenticator(a)
			rapt, err := ra.GetRAPT(ctx)
			if err == nil {
				fmt.Printf("authtype=BearerReAuth\n")
				fmt.Printf("credential=%s:%s\n", t.AccessToken, rapt)
				os.Exit(0)
			}
			if bypassReAuth() {
				printBypassWarning()
				// Fall through to non-ReAuth case
			} else {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				fmt.Fprint(os.Stderr, reAuthRequiredMsg)
				os.Exit(1)
			}
		}
		// Fall through if ReAuth is not needed
	}
	fmt.Printf("username=git-luci\n")
	fmt.Printf("password=%s\n", t.AccessToken)
}

func checkReAuth(ctx context.Context, a *auth.Authenticator, opts auth.Options, attrs *creds.Attrs) (*gerrit.ReAuthCheckResult, error) {
	c, err := a.Client()
	if err != nil {
		return nil, errors.Fmt("checkReAuthNeeded: %s", err)
	}
	cache := getReAuthResultCache(ctx, opts)
	logging.Debugf(ctx, "Checking if ReAuth is needed")
	checker := gerrit.NewReAuthChecker(c, cache)
	res, err := checker.Check(ctx, attrs)
	if err != nil {
		return nil, errors.Fmt("checkReAuthNeeded: %s", err)
	}
	logging.Debugf(ctx, "Got ReAuth check result %+v", res)
	return res, nil
}

func getReAuthResultCache(ctx context.Context, opts auth.Options) gerrit.ReAuthResultCache {
	if dir := opts.SecretsDir; dir != "" {
		return gerrit.NewDiskResultCache(ctx, dir)
	} else {
		return &gerrit.MemResultCache{}
	}
}

func printBypassWarning() {
	fmt.Fprintf(os.Stderr, `
!!!!!!!!!!!!!!!!!!!!!!!!!!!!! WARNING !!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!
You are bypassing ReAuth for a Gerrit repo that will enforce ReAuth
soon.

You can stop bypassing by removing the LUCI_BYPASS_REAUTH environment
variable.

If you are bypassing ReAuth due to an issue and have not already filed
a bug, please do so at:
https://issues.chromium.org/issues/new?component=1456702&template=2176581
------------------------------------------------------------------------
`)
}

func debugLogEnabled() bool {
	return os.Getenv("LUCI_AUTH_DEBUG") != ""
}

func reAuthEnabled() bool {
	return os.Getenv("LUCI_ENABLE_REAUTH") != ""
}

// forceReAuth will force providing a RAPT even if Gerrit reports that
// a RAPT isn't needed for the repo.  This is for testing and working
// around bugs.
func forceReAuth() bool {
	return os.Getenv("LUCI_FORCE_REAUTH") != ""
}

// bypassReAuth bypasses the client side ReAuth enforcement.
//
// Specifically, if the Gerrit host reports a ReAuth requirement, we
// will normally fail if we do not have a valid RAPT.
//
// This bypass will make us fall back to providing credentials without
// a RAPT instead of failing.
//
// Naturally, this will only work if the ReAuth isn't enforced server
// side.
//
// (The server reporting a requirement does not necessarily mean the
// server enforces it, e.g., during rollout.)
func bypassReAuth() bool {
	return os.Getenv("LUCI_BYPASS_REAUTH") != ""
}
