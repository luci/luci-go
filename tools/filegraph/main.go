package main

import (
	"context"
	"fmt"
	"os"

	"github.com/maruel/subcommands"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/flag/fixflagpos"
	"go.chromium.org/luci/common/logging/gologger"
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

func main() {
	app := &cli.Application{
		Name:  "filegraph",
		Title: "Filegraph.",
		Context: func(ctx context.Context) context.Context {
			return logCfg.Use(ctx)
		},
		Commands: []*subcommands.Command{
			cmdStats,
			cmdPath,
			cmdQuery,
			// cmdAdd(p),
			// cmdGet(p),
			// cmdLS(p),
			// cmdLog(p),
			// cmdCancel(p),
			// cmdBatch(p),
			// cmdCollect(p),

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
