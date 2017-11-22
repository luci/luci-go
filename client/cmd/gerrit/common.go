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
	"net/http"

	"github.com/maruel/subcommands"
	"golang.org/x/net/context"

	"go.chromium.org/luci/client/authcli"
	"go.chromium.org/luci/client/internal/common"
	"go.chromium.org/luci/common/auth"
	"go.chromium.org/luci/common/logging/gologger"
)

type commonFlags struct {
	subcommands.CommandRunBase
	defaultFlags   common.Flags
	authFlags      authcli.Flags
	parsedAuthOpts auth.Options
	jsonOutput     string
}

func (c *commonFlags) Init(authOpts auth.Options) {
	c.defaultFlags.Init(&c.Flags)
	c.authFlags.Register(&c.Flags, authOpts)
	c.Flags.StringVar(&c.jsonOutput, "json-output", "", "Path to write operation results to (use '-' for stdout).")
}

func (c *commonFlags) Parse() error {
	var err error
	if err = c.defaultFlags.Parse(); err != nil {
		return err
	}
	c.parsedAuthOpts, err = c.authFlags.Options()
	return err
}

func (c *commonFlags) createAuthClient() (*http.Client, error) {
	ctx := gologger.StdConfig.Use(context.Background())
	return auth.NewAuthenticator(ctx, auth.SilentLogin, c.parsedAuthOpts).Client()
}
