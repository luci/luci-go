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
	"fmt"
	"os"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/errors"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/grpcutil"
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
	switch err := c.commonFlags.Parse(); {
	case err != nil:
		return err
	case len(args) != 2:
		return errors.New("exactly 2 position arguments are expected")
	default:
		return nil
	}
}

func (c *refsRun) main(a subcommands.Application, args []string) error {
	ctx := c.defaultFlags.MakeLoggingContext(os.Stderr)

	host, project, err := gitiles.ParseRepoURL(args[0])
	if err != nil {
		return errors.Fmt("invalid repo URL %q: %w", args[0], err)
	}

	authCl, err := c.createAuthClient()
	if err != nil {
		return err
	}
	g, err := gitiles.NewRESTClient(authCl, host, true)
	if err != nil {
		return err
	}

	var res *gitilespb.RefsResponse
	if err := retry.Retry(ctx, transient.Only(retry.Default), func() error {
		var err error
		res, err = g.Refs(ctx, &gitilespb.RefsRequest{
			Project:  project,
			RefsPath: args[1],
		})
		return grpcutil.WrapIfTransient(err)
	}, nil); err != nil {
		return err
	}

	if c.jsonOutput == "" {
		for k, v := range res.Revisions {
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

	data, err := json.MarshalIndent(res.Revisions, "", "  ")
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
