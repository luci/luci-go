// Copyright 2019 The LUCI Authors.
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

func cmdDownloadFile(authOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "download-file <options> repository-url committish path",
		ShortDesc: "prints the contents of the file at path in the repository",
		LongDesc:  "Prints the contents of the file at path in the repository at requested commit.",
		CommandRun: func() subcommands.CommandRun {
			c := downloadFileRun{}
			c.commonFlags.Init(authOpts)
			return &c
		},
	}
}

type downloadFileRun struct {
	commonFlags
}

func (c *downloadFileRun) Parse(a subcommands.Application, args []string) error {
	switch err := c.commonFlags.Parse(); {
	case err != nil:
		return err
	case len(args) != 3:
		return errors.New("exactly 3 position arguments are expected")
	default:
		return nil
	}
}

func (c *downloadFileRun) main(a subcommands.Application, args []string) error {
	ctx := c.defaultFlags.MakeLoggingContext(os.Stderr)

	repoURL := args[0]
	committish := args[1]
	path := args[2]

	host, project, err := gitiles.ParseRepoURL(repoURL)
	if err != nil {
		return errors.Fmt("invalid repo URL %q: %w", args[0], err)
	}

	req := &gitilespb.DownloadFileRequest{
		Project:    project,
		Committish: committish,
		Path:       path,
	}

	authCl, err := c.createAuthClient()
	if err != nil {
		return err
	}
	g, err := gitiles.NewRESTClient(authCl, host, true)
	if err != nil {
		return err
	}

	var res *gitilespb.DownloadFileResponse
	if err := retry.Retry(ctx, transient.Only(retry.Default), func() error {
		var err error
		res, err = g.DownloadFile(ctx, req)
		return grpcutil.WrapIfTransient(err)
	}, nil); err != nil {
		return err
	}

	if res.Contents == "" {
		fmt.Fprintf(a.GetErr(), "warning: file %s is empty", path)
	}
	fmt.Fprintf(a.GetOut(), "%s", res.Contents)
	return nil
}

func (c *downloadFileRun) Run(a subcommands.Application, args []string, _ subcommands.Env) int {
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
