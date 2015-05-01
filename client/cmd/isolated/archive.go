// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"os"
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
	start := time.Now()
	interrupt.HandleCtrlC()
	arch := archiver.New(isolatedclient.New(c.serverURL, c.namespace))
	futures := []archiver.Future{}
	names := []string{}
	for _, file := range c.files {
		futures = append(futures, arch.PushFile(file, file))
		names = append(names, file)
	}

	for _, d := range c.dirs {
		futures = append(futures, archiver.PushDirectory(arch, d, nil))
		names = append(names, d)
	}

	for i, future := range futures {
		future.WaitForHashed()
		if err := future.Error(); err == nil {
			fmt.Printf("%s  %s\n", future.Digest(), names[i])
		} else {
			fmt.Printf("%s failed: %s\n", names[i], err)
		}
	}
	// This waits for all uploads.
	err := arch.Close()
	duration := time.Now().Sub(start)
	stats := arch.Stats()

	fmt.Fprintf(os.Stderr, "Hits    : %5d (%.1fkb)\n", len(stats.Hits), float64(stats.TotalHits())/1024.)
	fmt.Fprintf(os.Stderr, "Misses  : %5d (%.1fkb)\n", len(stats.Misses), float64(stats.TotalMisses())/1024.)
	fmt.Fprintf(os.Stderr, "Pushed  : %5d (%.1fkb)\n", len(stats.Pushed), float64(stats.TotalPushed())/1024.)
	fmt.Fprintf(os.Stderr, "Duration: %s\n", duration)
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
