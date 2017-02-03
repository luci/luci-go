// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

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
	// TODO(vadimsh): This is copy-pasta of createAuthClient from
	// cmd/isolate/common.go.
	authOpts := c.parsedAuthOpts
	var loginMode auth.LoginMode
	if authOpts.ServiceAccountJSONPath != "" {
		authOpts.Method = auth.ServiceAccountMethod
		loginMode = auth.SilentLogin
	} else {
		authOpts.Method = auth.UserCredentialsMethod
		loginMode = auth.OptionalLogin
	}
	ctx := gologger.StdConfig.Use(context.Background())
	return auth.NewAuthenticator(ctx, loginMode, authOpts).Client()
}
