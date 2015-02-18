// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"fmt"

	"chromium.googlesource.com/infra/swarming/client-go/swarming"
	"github.com/kr/pretty"
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

func (c *requestShowRun) main(a subcommands.Application, taskid string) error {
	if err := c.Parse(a); err != nil {
		return err
	}
	s, err := swarming.NewSwarming(c.serverURL)
	if err != nil {
		return err
	}
	r, err := s.FetchRequest(swarming.TaskID(taskid))
	if err != nil {
		return fmt.Errorf("failed to load task %s: %s", taskid, err)
	}
	_ = pretty.Println(r)
	return err
}

func (c *requestShowRun) Run(a subcommands.Application, args []string) int {
	if len(args) != 1 {
		fmt.Fprintf(a.GetErr(), "%s: Must only provide a task id.\n", a.GetName())
		return 1
	}
	if err := c.main(a, args[0]); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	return 0
}
