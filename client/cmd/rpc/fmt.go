// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"github.com/maruel/subcommands"
)

var cmdFmt = &subcommands.Command{
	UsageLine: `fmt subcommand [arguments]`,
	ShortDesc: "converts a message to/from flagpb and JSON formats",
	LongDesc:  "Converts a message to/from flagpb and JSON formats.",
	CommandRun: func() subcommands.CommandRun {
		c := &fmtRun{}
		return c
	},
}

type fmtRun struct {
	cmdRun
}

func (r *fmtRun) Run(a subcommands.Application, args []string) int {
	app := &subcommands.DefaultApplication{
		Name:  "fmt",
		Title: "Converts a message formats.",
		Commands: []*subcommands.Command{
			cmdJ2F,
			cmdF2J,
		},
	}
	return subcommands.Run(app, args)
}
