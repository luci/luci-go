// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/luci/luci-go/client/archiver"
	"github.com/luci/luci-go/client/internal/common"
	"github.com/luci/luci-go/client/isolatedclient"
	"github.com/maruel/interrupt"
	"github.com/maruel/subcommands"
)

var cmdArchive = &subcommands.Command{
	UsageLine: "archive <options>...",
	ShortDesc: "creates a .isolated file and uploads the tree to an isolate server.",
	LongDesc:  "All the files listed in the .isolated file are put in the isolate server.",
	CommandRun: func() subcommands.CommandRun {
		c := archiveRun{}
		c.commonFlags.Init(&c.CommandRunBase)
		c.commonServerFlags.Init(&c.CommandRunBase)
		c.Flags.Var(&c.dirs, "dirs", "Directory(ies) to archive")
		c.Flags.Var(&c.files, "files", "Individual file(s) to archive")
		c.Flags.Var(&c.blacklist, "blacklist",
			"List of regexp to use as blacklist filter when uploading directories")
		return &c
	},
}

type archiveRun struct {
	subcommands.CommandRunBase
	commonFlags
	commonServerFlags
	dirs      common.Strings
	files     common.Strings
	blacklist common.Strings
}

func (c *archiveRun) Parse(a subcommands.Application, args []string) error {
	if err := c.commonFlags.Parse(); err != nil {
		return err
	}
	if err := c.commonServerFlags.Parse(); err != nil {
		return err
	}
	if len(args) != 0 {
		return errors.New("position arguments not expected")
	}
	return nil
}

func (c *archiveRun) main(a subcommands.Application, args []string) error {
	if len(c.dirs) != 0 {
		return errors.New("-dirs is not yet implemented")
	}
	start := time.Now()
	interrupt.HandleCtrlC()
	is := isolatedclient.New(c.serverURL, c.namespace)

	archiver := archiver.New(is)
	for _, file := range c.files {
		archiver.PushFile(file)
	}
	err := archiver.Close()
	duration := time.Now().Sub(start)
	stats := archiver.Stats()

	fmt.Printf("Hits    : %5d (%.1fkb)\n", len(stats.Hits), float64(stats.TotalHits())/1024.)
	fmt.Printf("Misses  : %5d (%.1fkb)\n", len(stats.Misses), float64(stats.TotalMisses())/1024.)
	fmt.Printf("Pushed  : %5d (%.1fkb)\n", len(stats.Pushed), float64(stats.TotalPushed())/1024.)
	fmt.Printf("Duration: %s\n", duration)
	return err
}

func (c *archiveRun) Run(a subcommands.Application, args []string) int {
	if err := c.Parse(a, args); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	if err := c.main(a, args); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	return 0
}
