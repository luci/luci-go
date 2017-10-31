// Copyright 2017 The LUCI Authors.
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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/maruel/subcommands"
	"go.chromium.org/luci/common/auth"
	"go.chromium.org/luci/common/api/gitiles"
)

func cmdLog(authOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "log <options> repository treeish",
		ShortDesc: "prints commits based on a repo and treeish",
		LongDesc: `Prints commits based on a repo and treeish.

This should be equivalent of a "git log <treeish>" call in that repository.`,
		CommandRun: func() subcommands.CommandRun {
			c := logRun{}
			c.commonFlags.Init(authOpts)
			c.Flags.IntVar(&c.limit, "limit", 0, "Limit the number of log entries returned.")
			c.Flags.StringVar(&c.jsonOutput, "json-output", "", "Path to write operation results to.")
			return &c
		},
	}
}

type logRun struct {
	commonFlags
	limit int
	jsonOutput string
}

func (c *logRun) Parse(a subcommands.Application, args []string) error {
	if err := c.commonFlags.Parse(); err != nil {
		return err
	}
	if len(args) < 2 {
		return errors.New("position arguments missing")
	} else if len(args) > 2 {
		return errors.New("position arguments not expected")
	}
	return nil
}

func (c *logRun) main(a subcommands.Application, args []string) error {
	authCl, err := c.createAuthClient()
	if err != nil {
		return err
	}
	ctx := c.defaultFlags.MakeLoggingContext(os.Stderr)

	g := &gitiles.Client{Client: authCl}
	commits, err := g.Log(ctx, args[0], args[1], gitiles.Limit(c.limit))
	if err != nil {
		return err
	}

	if c.jsonOutput == "" {
		for _, c := range commits {
			fmt.Printf("%s %s\n", c.Commit[:7], strings.Split(c.Message, "\n")[0])
		}
	} else {
		out := os.Stdout
		if c.jsonOutput != "-" {
			out, err = os.Create(c.jsonOutput)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return err
			}
			defer out.Close()
		}
		if err = json.NewEncoder(out).Encode(commits); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return err
		}
	}
	return nil
}

func (c *logRun) Run(a subcommands.Application, args []string, _ subcommands.Env) int {
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
