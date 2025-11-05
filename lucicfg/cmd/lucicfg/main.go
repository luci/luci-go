// Copyright 2018 The LUCI Authors.
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

// These are still not the default in Go 1.26; lucicfg can handle tarballs via
// archive/tar (for gitilessource package fetching). Enabling the extra setting
// for zip files also seems prudent.
//
//go:debug tarinsecurepath=0
//go:debug zipinsecurepath=0

// Command lucicfg is CLI for LUCI config generator.
package main

import (
	"os"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/api/gerrit"
	"go.chromium.org/luci/hardcoded/chromeinfra"

	"go.chromium.org/luci/lucicfg/cli"
	"go.chromium.org/luci/lucicfg/cli/base"
)

func main() {
	authOpts := chromeinfra.DefaultAuthOptions()
	authOpts.Scopes = []string{
		auth.OAuthScopeEmail,
		gerrit.OAuthScope,
	}
	params := base.Parameters{
		AuthOptions:       authOpts,
		ConfigServiceHost: chromeinfra.ConfigServiceHost,
	}
	os.Exit(cli.Main(params, os.Args[1:]))
}
