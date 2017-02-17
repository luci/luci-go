// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package authutil

import (
	"github.com/maruel/subcommands"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/client/authcli"
	"github.com/luci/luci-go/common/auth"
	"github.com/luci/luci-go/common/cli"
	"github.com/luci/luci-go/common/logging/gologger"
)

// GetApplication returns cli.Application that implements 'authutil'.
//
// It does NOT hardcode any default values. Defaults are hardcoded in
// corresponding 'main' package.
func GetApplication(defaultAuthOpts auth.Options) *cli.Application {
	return &cli.Application{
		Name:  "authutil",
		Title: "LUCI Authentication Utility",
		Context: func(ctx context.Context) context.Context {
			return gologger.StdConfig.Use(ctx)
		},
		Commands: []*subcommands.Command{
			subcommands.CmdHelp,
			authcli.SubcommandInfoWithParams(authcli.CommandParams{
				Name:        "info",
				AuthOptions: defaultAuthOpts,
				ScopesFlag:  true,
			}),
			authcli.SubcommandLoginWithParams(authcli.CommandParams{
				Name:        "login",
				AuthOptions: defaultAuthOpts,
				ScopesFlag:  true,
			}),
			authcli.SubcommandLogoutWithParams(authcli.CommandParams{
				Name:        "logout",
				AuthOptions: defaultAuthOpts,
				ScopesFlag:  true,
			}),
			authcli.SubcommandTokenWithParams(authcli.CommandParams{
				Name:        "token",
				AuthOptions: defaultAuthOpts,
				ScopesFlag:  true,
			}),
			authcli.SubcommandContextWithParams(authcli.CommandParams{
				Name:        "context",
				Advanced:    true,
				AuthOptions: defaultAuthOpts,
				ScopesFlag:  true,
			}),
		},
	}
}
