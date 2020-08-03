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

// Package main is a CAS client.
//
// This is a thin wrapper of remote-apis-sdks to upload/download files from CAS
// efficiently.
package main

import (
	"context"
	"log"
	"os"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/client/cmd/cas/lib"
	"go.chromium.org/luci/client/versioncli"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

const version = "0.1"

func getApplication(defaultAuthOpts auth.Options) *cli.Application {
	return &cli.Application{
		Name:  "cas",
		Title: "Client tool to access CAS.",
		Context: func(ctx context.Context) context.Context {
			return gologger.StdConfig.Use(ctx)
		},
		Commands: []*subcommands.Command{
			subcommands.CmdHelp,

			lib.CmdArchive(),
			lib.CmdDownload(),

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
	app := getApplication(chromeinfra.DefaultAuthOptions())
	os.Exit(subcommands.Run(app, nil))
}
