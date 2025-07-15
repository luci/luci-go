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

package luci_auth

import (
	"context"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/client/versioncli"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/logging/gologger"
)

// Version is the version of luci-auth tool.
const Version = "1.5.7"

// GetApplication returns cli.Application that implements 'luci-auth'.
//
// It does NOT hardcode any default values. Defaults are hardcoded in
// corresponding 'main' package.
func GetApplication(defaultAuthOpts auth.Options) *cli.Application {
	return &cli.Application{
		Name:  "luci-auth",
		Title: "LUCI Authentication Utility (luci-auth v" + Version + ")",

		Context: func(ctx context.Context) context.Context {
			return gologger.StdConfig.Use(ctx)
		},

		Commands: []*subcommands.Command{
			subcommands.CmdHelp,
			versioncli.CmdVersion("luci-auth v" + Version),

			authcli.SubcommandLoginWithParams(authcli.CommandParams{
				Name:          "login",
				AuthOptions:   defaultAuthOpts,
				UseScopeFlags: true,
			}),
			authcli.SubcommandLogoutWithParams(authcli.CommandParams{
				Name:          "logout",
				AuthOptions:   defaultAuthOpts,
				UseScopeFlags: true,
			}),

			authcli.SubcommandInfoWithParams(authcli.CommandParams{
				Name:                     "info",
				AuthOptions:              defaultAuthOpts,
				UseScopeFlags:            true,
				UseIDTokenFlags:          true,
				UseCredentialHelperFlags: true,
				UseADCFlags:              true,
			}),
			authcli.SubcommandTokenWithParams(authcli.CommandParams{
				Name:                     "token",
				AuthOptions:              defaultAuthOpts,
				UseScopeFlags:            true,
				UseIDTokenFlags:          true,
				UseCredentialHelperFlags: true,
				UseADCFlags:              true,
			}),

			authcli.SubcommandContextWithParams(authcli.CommandParams{
				Name:                     "context",
				AuthOptions:              defaultAuthOpts,
				UseScopeFlags:            true,
				UseCredentialHelperFlags: true,
				UseADCFlags:              true,
			}),
		},
	}
}
