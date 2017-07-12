// Copyright 2014 The LUCI Authors.
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

// Command authutil can be used to interact with OAuth2 token cache on disk.
//
// It hardcodes chrome-infra specific defaults.
//
// Use "github.com/luci/luci-go/client/authcli/authutil" package to implement
// a binary with different defaults.
package main

import (
	"os"

	"github.com/maruel/subcommands"

	"github.com/luci/luci-go/client/authcli/authutil"
	"github.com/luci/luci-go/common/data/rand/mathrand"

	"github.com/luci/luci-go/hardcoded/chromeinfra"
)

func main() {
	mathrand.SeedRandomly()
	app := authutil.GetApplication(chromeinfra.DefaultAuthOptions())
	os.Exit(subcommands.Run(app, nil))
}
