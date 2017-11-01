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

	"github.com/maruel/subcommands"
	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/auth"
)

func cmdRefs(authOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "refs repository refspath",
		ShortDesc: "resolves each ref in a repo to git revision",
		LongDesc: `Resolves each ref in a repo to git revision.

refspath limits which refs to resolve to only those matching {refspath}/*.
refspath should start with "refs" and should not include glob '*'.
Typically, "refs/heads" should be used.`,
		CommandRun: func() subcommands.CommandRun {
			c := refsRun{}
			c.commonFlags.Init(authOpts)
			c.Flags.StringVar(&c.jsonOutput, "json-output", "", "Path to write operation results to.")
			return &c
		},
	}
}

type refsRun struct {
	commonFlags
	jsonOutput string
}

func (c *refsRun) Parse(a subcommands.Application, args []string) error {
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

func (c *refsRun) main(a subcommands.Application, args []string) error {
	authCl, err := c.createAuthClient()
	if err != nil {
		return err
	}
	ctx := c.defaultFlags.MakeLoggingContext(os.Stderr)

	g := &gitiles.Client{Client: authCl}
	refs, err := g.Refs(ctx, args[0], args[1])
	if err != nil {
		return err
	}

	if c.jsonOutput == "" {
		for k, v := range refs {
			fmt.Printf("%s        %s\n", v, k)
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
	data, err := json.MarshalIndent(refs, "", "  ")
	if err != nil {
		return err
	}
	_, err = out.Write(data)
	return err
}

func (c *refsRun) Run(a subcommands.Application, args []string, _ subcommands.Env) int {
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
