// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"errors"
	"fmt"

	"github.com/maruel/subcommands"
)

var cmdDownload = &subcommands.Command{
	UsageLine: "download <options>...",
	ShortDesc: "downloads a file or a .isolated tree from an isolate server.",
	LongDesc: `Downloads one or multiple files, or a isolated tree from the isolate server.

Files are referenced by their hash`,
	CommandRun: func() subcommands.CommandRun {
		c := downloadRun{}
		c.commonFlags.Init(&c.CommandRunBase)
		c.commonServerFlags.Init(&c.CommandRunBase)
		return &c
	},
}

type downloadRun struct {
	subcommands.CommandRunBase
	commonFlags
	commonServerFlags
}

func (c *downloadRun) Parse(a subcommands.Application, args []string) error {
	if err := c.commonServerFlags.Parse(); err != nil {
		return err
	}
	if len(args) != 0 {
		return errors.New("position arguments not expected")
	}
	return nil
}

func (c *downloadRun) main(a subcommands.Application, args []string) error {
	return errors.New("TODO")
}

func (c *downloadRun) Run(a subcommands.Application, args []string) int {
	defer c.Close()
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
