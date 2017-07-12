// Copyright 2015 The LUCI Authors.
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

	"github.com/luci/luci-go/client/authcli"
	"github.com/luci/luci-go/client/internal/common"
	"github.com/luci/luci-go/common/auth"
	"github.com/luci/luci-go/common/isolatedclient"
	"github.com/luci/luci-go/common/logging/gologger"
)

type commonFlags struct {
	subcommands.CommandRunBase
	defaultFlags   common.Flags
	isolatedFlags  isolatedclient.Flags
	authFlags      authcli.Flags
	parsedAuthOpts auth.Options
}

func (c *commonFlags) Init(authOpts auth.Options) {
	c.defaultFlags.Init(&c.Flags)
	c.isolatedFlags.Init(&c.Flags)
	c.authFlags.Register(&c.Flags, authOpts)
}

func (c *commonFlags) Parse() error {
	var err error
	if err = c.defaultFlags.Parse(); err != nil {
		return err
	}
	if err = c.isolatedFlags.Parse(); err != nil {
		return err
	}
	c.parsedAuthOpts, err = c.authFlags.Options()
	return err
}

func (c *commonFlags) createAuthClient() (*http.Client, error) {
	// Don't enforce authentication by using OptionalLogin mode. This is needed
	// for IP whitelisted bots: they have NO credentials to send.
	ctx := gologger.StdConfig.Use(context.Background())
	return auth.NewAuthenticator(ctx, auth.OptionalLogin, c.parsedAuthOpts).Client()
}
