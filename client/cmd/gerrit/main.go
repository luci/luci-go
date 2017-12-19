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

package main

import (
	"log"
	"os"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/client/authcli"
	"go.chromium.org/luci/client/versioncli"
	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/auth"
	"go.chromium.org/luci/common/data/rand/mathrand"

	"go.chromium.org/luci/hardcoded/chromeinfra"
)

// version must be updated whenever functional change (behavior, arguments,
// supported commands) is done.
const version = "0.1"

func GetApplication(defaultAuthOpts auth.Options) *subcommands.DefaultApplication {
	defaultAuthOpts.Scopes = []string{auth.OAuthScopeEmail, gitiles.OAuthScope}
	return &subcommands.DefaultApplication{
		Name:  "gerrit",
		Title: "gerrit client",
		// Keep in alphabetical order of their name.
		Commands: []*subcommands.Command{
			cmdChangeAbandon(defaultAuthOpts),
			cmdChangeCreate(defaultAuthOpts),
			cmdChangeDetail(defaultAuthOpts),
			cmdChangeQuery(defaultAuthOpts),
			cmdSetReview(defaultAuthOpts),
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
