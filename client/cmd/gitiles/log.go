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
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/golang/protobuf/jsonpb"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/errors"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/grpcutil"
)

func cmdLog(authOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "log <options> repository-url committish",
		ShortDesc: "prints commits based on a repo and committish",
		LongDesc: `Prints commits based on a repo and committish.

This should be equivalent of a "git log <committish>" call in that repository.`,
		CommandRun: func() subcommands.CommandRun {
			c := logRun{}
			c.commonFlags.Init(authOpts)
			c.Flags.IntVar(&c.limit, "limit", 0, "Limit the number of log entries returned.")
			c.Flags.BoolVar(&c.withTreeDiff, "with-tree-diff", false, "Return 'TreeDiff' information in the returned commits.")
			c.Flags.StringVar(
				&c.jsonOutput,
				"json-output",
				"",
				"Path to write operation results to. "+
					"Output is JSON array of JSONPB commits.")
			return &c
		},
	}
}

type logRun struct {
	commonFlags
	limit        int
	withTreeDiff bool
	jsonOutput   string
}

func (c *logRun) Parse(a subcommands.Application, args []string) error {
	switch err := c.commonFlags.Parse(); {
	case err != nil:
		return err
	case len(args) != 2:
		return errors.New("exactly 2 position arguments are expected")
	default:
		return nil
	}
}

func (c *logRun) main(a subcommands.Application, args []string) error {
	ctx := c.defaultFlags.MakeLoggingContext(os.Stderr)

	host, project, err := gitiles.ParseRepoURL(args[0])
	if err != nil {
		return errors.Annotate(err, "invalid repo URL %q", args[0]).Err()
	}

	req := &gitilespb.LogRequest{
		Project:  project,
		PageSize: int32(c.limit),
		TreeDiff: c.withTreeDiff,
	}

	switch commits := strings.SplitN(args[1], "..", 2); len(commits) {
	case 0:
		return errors.New("committish is required")
	case 1:
		req.Committish = commits[0]
	case 2:
		req.ExcludeAncestorsOf = commits[0]
		req.Committish = commits[1]
	default:
		panic("impossible")
	}

	authCl, err := c.createAuthClient()
	if err != nil {
		return err
	}
	g, err := gitiles.NewRESTClient(authCl, host, true)
	if err != nil {
		return err
	}

	var res *gitilespb.LogResponse
	if err := retry.Retry(ctx, transient.Only(retry.Default), func() error {
		var err error
		res, err = g.Log(ctx, req)
		return grpcutil.WrapIfTransient(err)
	}, nil); err != nil {
		return err
	}

	if c.jsonOutput == "" {
		for _, c := range res.Log {
			fmt.Printf("commit %s\n", c.Id)
			fmt.Printf("Author: %s <%s>\n", c.Author.Name, c.Author.Email)
			t := c.Author.Time.AsTime()
			fmt.Printf("Date:   %s\n\n", t.Format(time.UnixDate))
			for _, l := range strings.Split(c.Message, "\n") {
				fmt.Printf("    %s\n", l)
			}
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

	// Note: for historical reasons, output format is JSON array, so
	// marshal individual commits to JSONPB and then marshall all that
	// as a JSON array.
	arr := make([]json.RawMessage, len(res.Log))
	m := &jsonpb.Marshaler{}
	for i, c := range res.Log {
		buf := &bytes.Buffer{} // cannot reuse
		if err := m.Marshal(buf, c); err != nil {
			return errors.Annotate(err, "could not marshal commit %q to JSONPB", c.Id).Err()
		}
		arr[i] = json.RawMessage(buf.Bytes())
	}

	data, err := json.MarshalIndent(arr, "", "  ")
	if err != nil {
		return err
	}
	_, err = out.Write(data)
	return err
}

func (c *logRun) Run(a subcommands.Application, args []string, _ subcommands.Env) int {
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
