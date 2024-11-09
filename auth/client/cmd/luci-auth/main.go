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

// Command luci-auth can be used to interact with OAuth2 token cache on disk.
//
// It hardcodes chrome-infra specific defaults.
//
// Use "go.chromium.org/luci/auth/client/luci_auth" package to implement
// a binary with different defaults.
package main

import (
	"os"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth/client/luci_auth"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

func main() {
	app := luci_auth.GetApplication(chromeinfra.DefaultAuthOptions())
	os.Exit(subcommands.Run(app, nil))
}
