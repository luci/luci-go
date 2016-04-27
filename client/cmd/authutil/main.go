// Copyright 2014 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Command authutil can be used to interact with OAuth2 token cache on disk.
package main

import (
	"os"

	"github.com/maruel/subcommands"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/client/authcli"
	"github.com/luci/luci-go/common/auth"
	"github.com/luci/luci-go/common/cli"
	"github.com/luci/luci-go/common/logging/gologger"
)

func main() {
	application := &cli.Application{
		Name:  "authutil",
		Title: "LUCI Authentication Utility",
		Context: func(ctx context.Context) context.Context {
			return gologger.StdConfig.Use(ctx)
		},
		Commands: []*subcommands.Command{
			subcommands.CmdHelp,
			authcli.SubcommandInfo(auth.Options{}, "info"),
			authcli.SubcommandLogin(auth.Options{}, "login"),
			authcli.SubcommandLogout(auth.Options{}, "logout"),
			authcli.SubcommandToken(auth.Options{}, "token"),
		},
	}
	os.Exit(subcommands.Run(application, nil))
}
