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
const version = "0.3"

func GetApplication(defaultAuthOpts auth.Options) *subcommands.DefaultApplication {
	return &subcommands.DefaultApplication{
		Name:  "isolate",
		Title: "isolate.py but faster",
		// Keep in alphabetical order of their name.
		Commands: []*subcommands.Command{
			cmdArchive(defaultAuthOpts),
			cmdBatchArchive(defaultAuthOpts),
			cmdExpArchive(defaultAuthOpts),
			cmdCheck(),
			subcommands.CmdHelp,
			authcli.SubcommandInfo(defaultAuthOpts, "whoami", false),
			authcli.SubcommandLogin(defaultAuthOpts, "login", false),
			authcli.SubcommandLogout(defaultAuthOpts, "logout", false),
			versioncli.CmdVersion(version),
		},
	}
}

func main() {
	log.SetFlags(log.Lmicroseconds)
	mathrand.SeedRandomly()
	app := GetApplication(chromeinfra.DefaultAuthOptions())
	os.Exit(subcommands.Run(app, nil))
}
