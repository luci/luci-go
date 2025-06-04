// Copyright 2018 The LUCI Authors.
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
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	humanize "github.com/dustin/go-humanize"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/grpcutil"
)

func cmdArchive(authOpts auth.Options) *subcommands.Command {

	return &subcommands.Command{
		UsageLine: "archive <options> repository-url committish",
		ShortDesc: "downloads an archive at the given repo committish",
		LongDesc: `Downloads an archive of a repo or a given path in the repo at
committish.

This tool does not stream the archive, so the full contents are stored in
memory before being written to disk.
		`,
		CommandRun: func() subcommands.CommandRun {
			c := archiveRun{}
			c.commonFlags.Init(authOpts)
			c.Flags.StringVar(&c.rawFormat, "format", "GZIP",
				fmt.Sprintf("Format of the archive requested. One of %s", formatChoices()))
			c.Flags.StringVar(&c.output, "output", "", "Path to write archive to.")
			c.Flags.StringVar(&c.path, "path", "", "Relative path to the repo project root.")
			return &c
		},
	}
}

type archiveRun struct {
	commonFlags
	format gitilespb.ArchiveRequest_Format
	output string
	path   string

	rawFormat string
}

func (c *archiveRun) Parse(a subcommands.Application, args []string) error {
	if err := c.commonFlags.Parse(); err != nil {
		return err
	}
	if len(args) != 2 {
		return errors.New("exactly 2 position arguments are expected")
	}
	if c.format = parseFormat(c.rawFormat); c.format == gitilespb.ArchiveRequest_Invalid {
		return errors.New("invalid archive format requested")
	}
	return nil
}

func formatChoices() []string {
	cs := make([]string, 0, len(gitilespb.ArchiveRequest_Format_value))
	for k := range gitilespb.ArchiveRequest_Format_value {
		cs = append(cs, k)
	}
	sort.Strings(cs)
	return cs
}

func parseFormat(f string) gitilespb.ArchiveRequest_Format {
	return gitilespb.ArchiveRequest_Format(gitilespb.ArchiveRequest_Format_value[strings.ToUpper(f)])
}

func (c *archiveRun) main(a subcommands.Application, args []string) error {
	ctx := c.defaultFlags.MakeLoggingContext(os.Stderr)
	host, project, err := gitiles.ParseRepoURL(args[0])
	if err != nil {
		return errors.Fmt("invalid repo URL %q: %w", args[0], err)
	}
	ref := args[1]
	req := &gitilespb.ArchiveRequest{
		Format:  c.format,
		Project: project,
		Ref:     ref,
		Path:    c.path,
	}

	authCl, err := c.createAuthClient()
	if err != nil {
		return err
	}
	g, err := gitiles.NewRESTClient(authCl, host, true)
	if err != nil {
		return err
	}

	var res *gitilespb.ArchiveResponse
	if err := retry.Retry(ctx, transient.Only(retry.Default), func() error {
		var err error
		res, err = g.Archive(ctx, req)
		return grpcutil.WrapIfTransient(err)
	}, nil); err != nil {
		return err
	}

	return c.dumpArchive(ctx, res)
}

func (c *archiveRun) dumpArchive(ctx context.Context, res *gitilespb.ArchiveResponse) error {
	var oPath string
	switch {
	case c.output != "":
		oPath = c.output
	case res.Filename != "":
		oPath = res.Filename
	default:
		return errors.New("No output path specified and no suggested archive name from remote")
	}

	f, err := os.Create(oPath)
	if err != nil {
		return errors.Fmt("failed to open file to write archive: %w", err)
	}
	defer f.Close()

	l, err := f.Write(res.Contents)
	logging.Infof(ctx, "Archive written to %s (size: %s)", oPath, humanize.Bytes(uint64(l)))
	return err
}

func (c *archiveRun) Run(a subcommands.Application, args []string, _ subcommands.Env) int {
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
