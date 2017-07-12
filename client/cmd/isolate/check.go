// Copyright 2015 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/maruel/subcommands"
)

func cmdCheck() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "check <options>",
		ShortDesc: "checks that all the inputs are present and generates .isolated",
		LongDesc:  "",
		CommandRun: func() subcommands.CommandRun {
			c := checkRun{}
			c.commonFlags.Init()
			c.isolateFlags.Init(&c.Flags)
			return &c
		},
	}
}

type checkRun struct {
	commonFlags
	isolateFlags
}

func (c *checkRun) Parse(a subcommands.Application, args []string) error {
	if err := c.commonFlags.Parse(); err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := c.isolateFlags.Parse(cwd, RequireIsolatedFile|RequireIsolateFile); err != nil {
		return err
	}
	if len(args) != 0 {
		return errors.New("position arguments not expected")
	}
	return nil
}

func (c *checkRun) main(a subcommands.Application, args []string) error {
	if !c.defaultFlags.Quiet {
		fmt.Printf("Isolate:   %s\n", c.Isolate)
		fmt.Printf("Isolated:  %s\n", c.Isolated)
		fmt.Printf("Blacklist: %s\n", c.Blacklist)
		fmt.Printf("Config:    %s\n", c.ConfigVariables)
		fmt.Printf("Path:      %s\n", c.PathVariables)
		fmt.Printf("Extra:     %s\n", c.ExtraVariables)
	}
	return errors.New("TODO")
}

func (c *checkRun) Run(a subcommands.Application, args []string, _ subcommands.Env) int {
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
	if err := c.main(a, args); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	return 0
}
