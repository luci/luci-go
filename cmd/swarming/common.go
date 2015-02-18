// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"errors"
	"os"

	"chromium.googlesource.com/infra/swarming/client-go/internal/common"
	"github.com/maruel/subcommands"
)

type commonFlags struct {
	subcommands.CommandRunBase
	serverURL string
	verbose   bool
}

// Init initializes common flags.
func (c *commonFlags) Init() {
	c.Flags.StringVar(&c.serverURL, "server", os.Getenv("SWARMING_SERVER"), "Server URL; required. Set $SWARMING_SERVER to set a default.")
	c.Flags.BoolVar(&c.verbose, "verbose", false, "Enable logging.")
}

// Parse parses the common flags.
func (c *commonFlags) Parse(a subcommands.Application) error {
	if c.serverURL == "" {
		return errors.New("must provide -server")
	}
	s, err := common.URLToHTTPS(c.serverURL)
	if err != nil {
		return err
	}
	c.serverURL = s
	return nil
}
