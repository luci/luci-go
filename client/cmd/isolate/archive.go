// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"errors"
	"fmt"

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
	if err := c.isolateFlags.Parse(); err != nil {
		return err
	}
	if len(args) != 0 {
		return errors.New("position arguments not expected")
	}
	return nil
}

func (c *archiveRun) main(a subcommands.Application, args []string) error {
	fmt.Printf("Server:    %s\n", c.serverURL)
	fmt.Printf("Namespace: %s\n", c.namespace)
	fmt.Printf("Isolate:   %s\n", c.Isolate)
	fmt.Printf("Isolated:  %s\n", c.Isolated)
	fmt.Printf("Blacklist: %s\n", c.Blacklist)
	fmt.Printf("Config:    %s\n", c.ConfigVariables)
	fmt.Printf("Path:      %s\n", c.PathVariables)
	fmt.Printf("Extra:     %s\n", c.ExtraVariables)
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
