// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"errors"
	"fmt"

	"github.com/kr/pretty"
	"github.com/luci/luci-go/client/swarming"
	"github.com/maruel/subcommands"
)

var cmdRequestShow = &subcommands.Command{
	UsageLine: "request-show <task_id>",
	ShortDesc: "returns properties of a request",
	LongDesc:  "Returns the properties, what, when, by who, about a request on the Swarming server.",
	CommandRun: func() subcommands.CommandRun {
		r := &requestShowRun{}
		r.Init()
		return r
	},
}

type requestShowRun struct {
	commonFlags
}

func (c *requestShowRun) Parse(a subcommands.Application, args []string) error {
	if err := c.commonFlags.Parse(a); err != nil {
		return err
	}
	if len(args) != 1 {
		return errors.New("must only provide a task id")
	}
	return nil
}

func (c *requestShowRun) main(a subcommands.Application, taskid string) error {
	s, err := swarming.New(c.serverURL)
	if err != nil {
		return err
	}
	r, err := s.FetchRequest(swarming.TaskID(taskid))
	if err != nil {
		return fmt.Errorf("failed to load task %s: %s", taskid, err)
	}
	_, _ = pretty.Println(r)
	return err
}

func (c *requestShowRun) Run(a subcommands.Application, args []string) int {
	if err := c.Parse(a, args); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	cl, err := c.defaultFlags.StartTracing()
	if err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	defer cl.Close()
	if err := c.main(a, args[0]); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	return 0
}
