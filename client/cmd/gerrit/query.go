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
	"go.chromium.org/luci/common/api/gerrit"
	"go.chromium.org/luci/common/auth"
	"go.chromium.org/luci/common/flag/stringlistflag"
)

func cmdQuery(authOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "query <options> url query",
		ShortDesc: "queries Gerrit for changes",
		LongDesc: `Queries Gerrit for changes.

options is a list of strings like {"CURRENT_REVISION"} which tells Gerrit
to return non-default properties for Change. The supported strings for
options are listed in Gerrit's api documentation at the link below:
https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#list-changes`,
		CommandRun: func() subcommands.CommandRun {
			c := queryRun{}
			c.commonFlags.Init(authOpts)
			c.Flags.IntVar(&c.limit, "limit", 0, "Limit the number of changes returned in the response.")
			c.Flags.IntVar(&c.skip, "skip", 0, "Skip this many from the list of results.")
			c.Flags.Var(&c.options, "options", "Include these options in the query.")
			c.Flags.StringVar(&c.jsonOutput, "json-output", "", "Path to write operation results to.")
			return &c
		},
	}
}

type queryRun struct {
	commonFlags
	limit      int
	skip       int
	options    stringlistflag.Flag
	jsonOutput string
}

func (c *queryRun) Parse(a subcommands.Application, args []string) error {
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

func (c *queryRun) main(a subcommands.Application, args []string) error {
	authCl, err := c.createAuthClient()
	if err != nil {
		return err
	}
	ctx := c.defaultFlags.MakeLoggingContext(os.Stderr)

	url := args[0]
	query := args[1]

	g, err := gerrit.NewClient(authCl, url)
	if err != nil {
		return err
	}

	req := gerrit.ChangeQueryRequest{
		Query:   query,
		N:       c.limit,
		S:       c.skip,
		Options: c.options,
	}
	changes, _, err := g.ChangeQuery(ctx, req)
	if err != nil {
		return err
	}

	if c.jsonOutput == "" {
		for _, c := range changes {
			fmt.Printf("ID %s\n", c.ID)
			fmt.Printf("Updated: %s\n", c.Updated)
			fmt.Printf("Status:  %s\n", strings.ToLower(c.Status))
			fmt.Printf("Subject: %s\n\n", c.Subject)
		}
		return nil
	}

	out := os.Stdout
	if c.jsonOutput != "-" {
		out, err = os.Create(c.jsonOutput)
		if err != nil {
			return err
		}
		defer out.Close()
	}
	data, err := json.MarshalIndent(changes, "", "  ")
	if err != nil {
		return err
	}
	_, err = out.Write(data)
	return err
}

func (c *queryRun) Run(a subcommands.Application, args []string, _ subcommands.Env) int {
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
