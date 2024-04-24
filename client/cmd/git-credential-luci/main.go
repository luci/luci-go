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
//
// https://www.kernel.org/pub/software/scm/git/docs/technical/api-credentials.html#_credential_helpers
// https://www.kernel.org/pub/software/scm/git/docs/git-credential.html
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/common/logging/gologger"

	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

var (
	flags    authcli.Flags
	lifetime time.Duration
)

func init() {
	defaults := chromeinfra.DefaultAuthOptions()
	defaults.Scopes = []string{gitiles.OAuthScope, auth.OAuthScopeEmail}
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
const loginRequiredMsg = `not running with a service account and not logged in

If you are running this in a development environment, you can log in by running:

luci-auth login -scopes "https://www.googleapis.com/auth/userinfo.email https://www.googleapis.com/auth/gerritcodereview"

`

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
	a := auth.NewAuthenticator(ctx, auth.SilentLogin, opts)

	switch flag.Args()[0] {
	case "get":
		t, err := a.GetAccessToken(lifetime)
		if err != nil {
			if err == auth.ErrLoginRequired {
				fmt.Fprintf(os.Stderr, loginRequiredMsg)
			} else {
				fmt.Fprintf(os.Stderr, "cannot get access token: %v\n", err)
			}
			os.Exit(1)
		}
		fmt.Printf("username=git-luci\n")
		fmt.Printf("password=%s\n", t.AccessToken)
	case "erase":
		if err := a.PurgeCredentialsCache(); err != nil {
			fmt.Fprintf(os.Stderr, "cannot erase cache: %v\n", err)
			os.Exit(1)
		}
	default:
		// The specification for Git credential helper says: "If a helper
		// receives any other operation, it should silently ignore the
		// request."
	}
}
