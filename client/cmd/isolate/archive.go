// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/luci/luci-go/client/archiver"
	"github.com/luci/luci-go/client/internal/common"
	"github.com/luci/luci-go/client/isolate"
	"github.com/luci/luci-go/client/isolatedclient"
	"github.com/maruel/subcommands"
)

var cmdArchive = &subcommands.Command{
	UsageLine: "archive <options>",
	ShortDesc: "creates a .isolated file and uploads the tree to an isolate server.",
	LongDesc:  "All the files listed in the .isolated file are put in the isolate server cache via isolateserver.py.",
	CommandRun: func() subcommands.CommandRun {
		c := archiveRun{}
		c.commonFlags.Init(&c.CommandRunBase)
		c.commonServerFlags.Init(&c.CommandRunBase)
		c.isolateFlags.Init(&c.CommandRunBase)
		return &c
	},
}

type archiveRun struct {
	subcommands.CommandRunBase
	commonFlags
	commonServerFlags
	isolateFlags
}

func (c *archiveRun) Parse(a subcommands.Application, args []string) error {
	if err := c.commonServerFlags.Parse(); err != nil {
		return err
	}
	if err := c.isolateFlags.Parse(RequireIsolatedFile); err != nil {
		return err
	}
	if len(args) != 0 {
		return errors.New("position arguments not expected")
	}
	return nil
}

func (c *archiveRun) main(a subcommands.Application, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	start := time.Now()
	arch := archiver.New(isolatedclient.New(c.serverURL, c.namespace))
	common.CancelOnCtrlC(arch)
	future := isolate.Archive(arch, cwd, &c.ArchiveOptions)
	future.WaitForHashed()
	if err = future.Error(); err != nil {
		fmt.Printf("%s  %s\n", filepath.Base(c.Isolate), err)
	} else {
		fmt.Printf("%s  %s\n", future.Digest(), filepath.Base(c.Isolate))
	}
	if err2 := arch.Close(); err == nil {
		err = err2
	}
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
