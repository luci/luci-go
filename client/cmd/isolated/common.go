// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"net/http"
	"runtime"

	"github.com/luci/luci-go/client/internal/common"
	"github.com/luci/luci-go/client/isolatedclient"
	"github.com/luci/luci-go/common/auth"
	"github.com/maruel/subcommands"
)

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
}

type commonFlags struct {
	subcommands.CommandRunBase
	defaultFlags  common.Flags
	isolatedFlags isolatedclient.Flags
}

func (c *commonFlags) Init() {
	c.defaultFlags.Init(&c.Flags)
	c.isolatedFlags.Init(&c.Flags)
}

func (c *commonFlags) Parse() error {
	if err := c.defaultFlags.Parse(); err != nil {
		return err
	}
	return c.isolatedFlags.Parse()
}

func (c *commonFlags) createClient() *http.Client {
	client, _ := auth.NewAuthenticator(auth.SilentLogin, auth.Options{}).Client()
	return client
}
