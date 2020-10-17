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

package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/flag/fixflagpos"
	"go.chromium.org/luci/common/logging/gologger"

	"go.chromium.org/luci/rts/filegraph/internal/git"
)

var logCfg = gologger.LoggerConfig{
	Format: `%{message}`,
	Out:    os.Stderr,
}

type baseCommandRun struct {
	subcommands.CommandRunBase
}

func (r *baseCommandRun) done(err error) int {
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	return 0
}

// Main runs the filegraph program.
func Main() {
	app := &cli.Application{
		Name:  "filegraph",
		Title: "Filegraph.",
		Context: func(ctx context.Context) context.Context {
			return logCfg.Use(ctx)
		},
		Commands: []*subcommands.Command{
			//cmdStats,
			cmdPath,
			cmdQuery,

			// {},
			// authcli.SubcommandLogin(p.Auth, "auth-login", false),
			// authcli.SubcommandLogout(p.Auth, "auth-logout", false),
			// authcli.SubcommandInfo(p.Auth, "auth-info", false),

			{},
			subcommands.CmdHelp,
		},
	}

	os.Exit(subcommands.Run(app, fixflagpos.FixSubcommands(os.Args[1:])))
}

func ensureSameRepo(files ...string) (gitPath string, err error) {
	for _, f := range files {
		switch gp, err := git.AbsoluteGitPath(f); {
		case err != nil:
			return "", err

		case gitPath == "":
			gitPath = gp
		case gitPath != gp:
			return "", errors.Reason("%q and %q reside in different git repositories", files[0], f).Err()
		}
	}
	return gitPath, nil
}
