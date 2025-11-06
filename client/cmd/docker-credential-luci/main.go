// Copyright 2018 The LUCI Authors.
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

// Command docker-credential-luci is a Docker credential helper.
//
// The protocol used for communication between Docker and the credential
// helper is heavily inspired by Git, but it differs in the information
// shared:
//
// https://docs.docker.com/engine/reference/commandline/login/#credential-helper-protocol
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/scopes"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: docker-credential-luci <command>\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	args := flag.Args()
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "docker-credential-luci: expecting only one argument, got %d\n", len(args))
		os.Exit(1)
	}

	ctx := gologger.StdConfig.Use(context.Background())

	opts := chromeinfra.DefaultAuthOptions()
	opts.Scopes = scopes.CloudScopeSet()
	auth := auth.NewAuthenticator(ctx, auth.SilentLogin, opts)

	// We only use the command and ignore the payload in stdin. Still need to
	// close it, otherwise it is possible (though very unlikely) docker CLI will
	// get stuck writing to a pipe no one is reading.
	os.Stdin.Close()

	switch args[0] {
	case "get":
		t, err := auth.GetAccessToken(3 * time.Minute)
		if err != nil {
			printErr("cannot get access token", err)
			os.Exit(1)
		}
		response := map[string]string{
			"Username": "oauth2accesstoken",
			"Secret":   t.AccessToken,
		}
		if err := json.NewEncoder(os.Stdout).Encode(&response); err != nil {
			printErr("cannot encode payload", err)
			os.Exit(1)
		}
	case "erase":
		if err := auth.PurgeCredentialsCache(); err != nil {
			printErr("cannot erase cache", err)
			os.Exit(1)
		}
	default:
		// We don't support the "store" and "list" commands and hence ignore them.
	}
}

func printErr(prefix string, err error) {
	switch {
	case err == auth.ErrLoginRequired:
		fmt.Fprintln(os.Stderr, "docker-credential-luci: not running with a service account and not logged in")
	case err != nil:
		fmt.Fprintf(os.Stderr, "docker-credential-luci: %s: %v\n", prefix, err)
	}
}
