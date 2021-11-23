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
	"strings"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/errors"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/grpcutil"
)

func cmdProjects(authOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "projects host",
		ShortDesc: "list all available projects",
		LongDesc: `List all available Gitiles projects.

Example: projects chromium.googlesource.com`,

		CommandRun: func() subcommands.CommandRun {
			c := projectsRun{}
			c.commonFlags.Init(authOpts)
			c.Flags.StringVar(&c.jsonOutput, "json-output", "", "Path to write operation results to.")
			return &c
		},
	}
}

type projectsRun struct {
	commonFlags
	jsonOutput string
}

func (c *projectsRun) Parse(a subcommands.Application, args []string) error {
	switch err := c.commonFlags.Parse(); {
	case err != nil:
		return err
	case len(args) != 1:
		return errors.New("host argument is expected")
	default:
		return nil
	}
}

func (c *projectsRun) main(a subcommands.Application, args []string) error {
	ctx := c.defaultFlags.MakeLoggingContext(os.Stderr)

	if strings.Index(args[0], "/") != -1 {
		return fmt.Errorf("host mustn't contain any slashes %s", args[0])
	}

	authCl, err := c.createAuthClient()
	if err != nil {
		return err
	}
	g, err := gitiles.NewRESTClient(authCl, args[0], true)
	if err != nil {
		return err
	}

	var res *gitilespb.ProjectsResponse
	if err := retry.Retry(ctx, transient.Only(retry.Default), func() error {
		var err error
		res, err = g.Projects(ctx, &gitilespb.ProjectsRequest{})
		return grpcutil.WrapIfTransient(err)
	}, nil); err != nil {
		return err
	}

	if c.jsonOutput == "" {
		for _, project := range res.Projects {
			fmt.Println(project)
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

	data, err := json.MarshalIndent(res.Projects, "", "  ")
	if err != nil {
		return err
	}
	_, err = out.Write(data)
	return err
}

func (c *projectsRun) Run(a subcommands.Application, args []string, _ subcommands.Env) int {
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
