// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"chromium.googlesource.com/infra/swarming/client-go/internal/common"
	"chromium.googlesource.com/infra/swarming/client-go/isolate"
	"github.com/maruel/subcommands"
)

type commonFlags struct {
	subcommands.CommandRunBase
	verbose bool
	logFile string
	noLog   bool
}

func (c *commonFlags) Init() {
	c.Flags.BoolVar(&c.verbose, "verbose", false, "Get more output") // ignores multiple times.
	c.Flags.StringVar(&c.logFile, "log-file", "", "Name of log file")
	c.Flags.BoolVar(&c.noLog, "no-log", false, "Disable log file.")
}

type authFlags struct {
	commonFlags
	authMethod string
	// TODO: other oauth options
}

func (c *authFlags) Init() {
	c.commonFlags.Init()
	c.Flags.StringVar(&c.authMethod, "auth", "oauth",
		"Authentication method to use: oauth, bot, none.")
}

type IsolateOptions struct {
	isolate.ArchiveOptions
	blacklistCollector  *common.NStringsCollect
	configVarsCollector *common.NKVArgCollect
	pathVarsCollector   *common.NKVArgCollect
	extraVarsCollector  *common.NKVArgCollect
}

func newIsolateOptions(b *subcommands.CommandRunBase) IsolateOptions {
	c := IsolateOptions{
		ArchiveOptions:      isolate.NewArchiveOptions(),
		blacklistCollector:  nil,
		configVarsCollector: nil,
		pathVarsCollector:   nil,
		extraVarsCollector:  nil}

	c.blacklistCollector = &common.NStringsCollect{&c.Blacklist}
	t := common.NewNKVArgCollect(&c.ConfigVariables, "config-variable")
	c.configVarsCollector = &t
	t = common.NewNKVArgCollect(&c.PathVariables, "path-variable")
	c.pathVarsCollector = &t
	t = common.NewNKVArgCollect(&c.ExtraVariables, "extra-variable")
	c.extraVarsCollector = &t

	b.Flags.StringVar(&c.ArchiveOptions.Subdir, "subdir", "",
		`Filters to a subdirectory. Its behavior changes depending if it
is a relative path as a string or as a path variable. Path
variables are always keyed from the directory containing the
.isolate file. Anything else is keyed on the root directory.`)
	b.Flags.StringVar(&c.ArchiveOptions.Isolate, "isolate", "",
		".isolate file to load the dependency data from")
	b.Flags.StringVar(&c.ArchiveOptions.Isolated, "isolated", "",
		".isolated file to generate or read")
	b.Flags.BoolVar(&c.ArchiveOptions.IgnoreBrokenItems, "ignore_broken_items", false,
		`Indicates that invalid entries in the isolated file to
be only be logged and not stop processing. Defaults to
True if env var ISOLATE_IGNORE_BROKEN_ITEMS is set`)

	b.Flags.Var(c.blacklistCollector, "blacklist",
		"List of regexp to use as blacklist filter when uploading directories")

	b.Flags.Var(c.configVarsCollector, "config-variable",
		`Config variables are used to determine which
conditions should be matched when loading a .isolate
file, default: []. All 3 kinds of variables are
persistent accross calls, they are saved inside
<.isolated>.state`)

	b.Flags.Var(c.pathVarsCollector, "path-variable",
		`Path variables are used to replace file paths when
	loading a .isolate file, default: []`)

	b.Flags.Var(c.extraVarsCollector, "extra-variable",
		`Extraneous variables are replaced on the 'command
entry and on paths in the .isolate file but are not
considered relative paths.`)
	return c
}
