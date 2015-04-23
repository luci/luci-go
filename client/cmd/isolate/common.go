// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/luci/luci-go/client/internal/common"
	"github.com/luci/luci-go/client/isolate"
	"github.com/maruel/subcommands"
)

type commonFlags struct {
	verbose bool
	logFile string
	noLog   bool
}

func (c *commonFlags) Init(b *subcommands.CommandRunBase) {
	b.Flags.BoolVar(&c.verbose, "verbose", false, "Get more output")
	b.Flags.StringVar(&c.logFile, "log", "", "Name of log file")
}

type commonServerFlags struct {
	serverURL string
	namespace string
}

func (c *commonServerFlags) Init(b *subcommands.CommandRunBase) {
	i := os.Getenv("ISOLATE_SERVER")
	b.Flags.StringVar(&c.serverURL, "isolate-server", i,
		"Isolate server to use; defaults to value of $ISOLATE_SERVER")
	b.Flags.StringVar(&c.serverURL, "I", i, "Alias for -isolate-server")
	b.Flags.StringVar(&c.namespace, "namespace", "testing", "")
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

type isolateFlags struct {
	// TODO(tandrii): move ArchiveOptions from isolate pkg to here.
	isolate.ArchiveOptions
}

func (c *isolateFlags) Init(b *subcommands.CommandRunBase) {
	c.ArchiveOptions.Init()
	b.Flags.StringVar(&c.Isolate, "isolate", "",
		".isolate file to load the dependency data from")
	b.Flags.StringVar(&c.Isolate, "i", "", "Alias for --isolate")
	b.Flags.StringVar(&c.Isolated, "isolated", "",
		".isolated file to generate or read")
	b.Flags.StringVar(&c.Isolated, "s", "", "Alias for --isolated")
	b.Flags.Var(&c.Blacklist, "blacklist",
		"List of regexp to use as blacklist filter when uploading directories")
	b.Flags.Var(c.ConfigVariables, "config-variable",
		`Config variables are used to determine which
		conditions should be matched when loading a .isolate
		file, default: []. All 3 kinds of variables are
		persistent accross calls, they are saved inside
		<.isolated>.state`)
	b.Flags.Var(c.PathVariables, "path-variable",
		`Path variables are used to replace file paths when
		loading a .isolate file, default: {}`)

	if common.IsWindows() {
		c.ExtraVariables["EXECUTABLE_SUFFIX"] = ".exe"
	}
	b.Flags.Var(c.ExtraVariables, "extra-variable",
		`Extraneous variables are replaced on the 'command
		entry and on paths in the .isolate file but are not
		considered relative paths.`)
}

func (c *isolateFlags) Parse() error {
	varss := [](common.KeyValVars){c.ConfigVariables, c.ExtraVariables, c.PathVariables}
	for _, vars := range varss {
		for k := range vars {
			if !isolate.IsValidVariable(k) {
				return fmt.Errorf("invalid key %s", k)
			}
		}
	}
	return nil
}
