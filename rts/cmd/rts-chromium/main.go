// Copyright 2020 The LUCI Authors.
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

package main

import (
	"context"
	"fmt"
	"os"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/storage"
	"github.com/maruel/subcommands"
	"google.golang.org/api/option"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/common/api/gerrit"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/flag/fixflagpos"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

var logCfg = gologger.LoggerConfig{
	Format: `%{message}`,
	Out:    os.Stderr,
}

func main() {
	mathrand.SeedRandomly()
	authOpt := chromeinfra.DefaultAuthOptions()
	authOpt.Scopes = append(authOpt.Scopes, bigquery.Scope, gerrit.OAuthScope, storage.ScopeReadOnly)

	app := &cli.Application{
		Name:  "rts-chromium",
		Title: "RTS for Chromium.",
		Context: func(ctx context.Context) context.Context {
			return logCfg.Use(ctx)
		},
		Commands: []*subcommands.Command{
			cmdPresubmitHistory(&authOpt),
			cmdFetchDurations(&authOpt),
			cmdCreateModel(&authOpt),
			cmdSelect(),

			{}, // a separator
			authcli.SubcommandLogin(authOpt, "auth-login", false),
			authcli.SubcommandLogout(authOpt, "auth-logout", false),
			authcli.SubcommandInfo(authOpt, "auth-info", false),

			{}, // a separator
			subcommands.CmdHelp,
		},
	}

	os.Exit(subcommands.Run(app, fixflagpos.FixSubcommands(os.Args[1:])))
}

type baseCommandRun struct {
	subcommands.CommandRunBase
}

func (r *baseCommandRun) done(err error) int {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func newBQClient(ctx context.Context, auth *auth.Authenticator) (*bigquery.Client, error) {
	http, err := auth.Client()
	if err != nil {
		return nil, err
	}
	return bigquery.NewClient(ctx, "chrome-rts", option.WithHTTPClient(http))
}
