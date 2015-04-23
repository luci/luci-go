// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"errors"
	"io/ioutil"
	"log"

	"github.com/luci/luci-go/client/internal/common"
	"github.com/maruel/subcommands"
)

type commonFlags struct {
	verbose bool
	logFile string
	noLog   bool
}

func (c *commonFlags) Init(b *subcommands.CommandRunBase) {
	b.Flags.BoolVar(&c.verbose, "verbose", false, "Get more output")
	// TODO(maruel): Implement -log.
	//b.Flags.StringVar(&c.logFile, "log", "", "Name of log file")
}

func (c *commonFlags) Parse() error {
	if !c.verbose {
		log.SetFlags(0)
		log.SetOutput(ioutil.Discard)
	}
	// TODO(maruel): Implement -log.
	return nil
}

type commonServerFlags struct {
	serverURL   string
	namespace   string
	compression string
	hashing     string
}

func (c *commonServerFlags) Init(b *subcommands.CommandRunBase) {
	b.Flags.StringVar(&c.serverURL, "isolate-server", "", "Isolate server to use")
	b.Flags.StringVar(&c.serverURL, "I", "", "Alias for -isolate-server")
	b.Flags.StringVar(&c.namespace, "namespace", "default-gzip", "")
}

func (c *commonServerFlags) Parse() error {
	if c.serverURL == "" {
		return errors.New("-isolate-server must be specified")
	}
	if s, err := common.URLToHTTPS(c.serverURL); err != nil {
		return err
	} else {
		c.serverURL = s
	}
	if c.namespace == "" {
		return errors.New("-namespace must be specified.")
	}
	return nil
}
