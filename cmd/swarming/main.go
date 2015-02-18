// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"log"
	"os"

	"github.com/maruel/subcommands"
)

var application = &subcommands.DefaultApplication{
	Name:  "swarming",
	Title: "Client tool to access a swarming server.",
	// Keep in alphabetical order of their name.
	Commands: []*subcommands.Command{
		subcommands.CmdHelp,
		cmdRequestShow,
	},
}

func main() {
	log.SetFlags(log.Lmicroseconds)
	os.Exit(subcommands.Run(application, nil))
}
