// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"errors"
	"fmt"

	"github.com/luci/luci-go/client/internal/common"
	"github.com/luci/luci-go/client/isolateserver"
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
	if err := c.commonServerFlags.Parse(); err != nil {
		return err
	}
	if len(args) != 0 {
		return errors.New("position arguments not expected")
	}
	return nil
}

func (c *archiveRun) main(a subcommands.Application, args []string) error {
	i := isolateserver.New(c.serverURL, c.namespace, c.compression, c.hashing)
	caps, err := i.ServerCapabilities()
	if err != nil {
		return err
	}

	fmt.Printf("Server:       %s\n", c.serverURL)
	fmt.Printf("Capabilities: %#v\n", caps)
	fmt.Printf("Namespace:    %s\n", c.namespace)
	fmt.Printf("Dirs:         %s\n", c.dirs)
	fmt.Printf("Files:        %s\n", c.files)
	fmt.Printf("Blacklist:    %s\n", c.blacklist)
	return errors.New("TODO")
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
