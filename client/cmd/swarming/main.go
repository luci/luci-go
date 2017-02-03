// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"log"
	"os"

	"github.com/maruel/subcommands"

	"github.com/luci/luci-go/client/authcli"
	"github.com/luci/luci-go/client/versioncli"
	"github.com/luci/luci-go/common/auth"
	"github.com/luci/luci-go/common/data/rand/mathrand"

	"github.com/luci/luci-go/hardcoded/chromeinfra"
)

// version must be updated whenever functional change (behavior, arguments,
// supported commands) is done.
const version = "0.2"

func GetApplication(defaultAuthOpts auth.Options) *subcommands.DefaultApplication {
	return &subcommands.DefaultApplication{
		Name:  "swarming",
		Title: "Client tool to access a swarming server.",
		// Keep in alphabetical order of their name.
		Commands: []*subcommands.Command{
			cmdRequestShow(defaultAuthOpts),
			cmdTrigger(defaultAuthOpts),
			subcommands.CmdHelp,
			authcli.SubcommandInfo(defaultAuthOpts, "whoami", false),
			authcli.SubcommandLogin(defaultAuthOpts, "login", false),
			authcli.SubcommandLogout(defaultAuthOpts, "logout", false),
			versioncli.CmdVersion(version),
		},

		EnvVars: map[string]subcommands.EnvVarDefinition{
			"SWARMING_TASK_ID": {
				Advanced: true,
				ShortDesc: ("Used when processing new triggered tasks. Is used as the " +
					"parent task ID for the newly triggered tasks."),
			},
		},
	}
}

func main() {
	log.SetFlags(log.Lmicroseconds)
	mathrand.SeedRandomly()
	app := GetApplication(chromeinfra.DefaultAuthOptions())
	os.Exit(subcommands.Run(app, nil))
}
