// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"log"
	"os"

	"github.com/luci/luci-go/client/internal/common"
	"github.com/maruel/subcommands"
)

// version must be updated whenever functional change (behavior, arguments,
// supported commands) is done.
const version = "0.1"

var application = &subcommands.DefaultApplication{
	Name:  "isolate",
	Title: "isolate.py but faster",
	// Keep in alphabetical order of their name.
	Commands: []*subcommands.Command{
		cmdArchive,
		cmdBatchArchive,
		cmdCheck,
		subcommands.CmdHelp,
		common.CmdVersion(version),
	},
}

func main() {
	log.SetFlags(log.Lmicroseconds)
	os.Exit(subcommands.Run(application, nil))
}
