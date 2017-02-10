// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"

	"github.com/maruel/subcommands"

	"github.com/luci/luci-go/common/api/buildbucket/buildbucket/v1"
	"github.com/luci/luci-go/common/auth"
	"github.com/luci/luci-go/common/cli"
)

func cmdPutBatch(defaultAuthOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `put [flags] <JSON request>...`,
		ShortDesc: "schedule builds",
		LongDesc: "Schedule builds. \n" +
			"See https://godoc.org/github.com/luci/luci-go/common/api/" +
			"buildbucket/buildbucket/v1#ApiPutBatchRequestMessage " +
			"for JSON request message schema.",
		CommandRun: func() subcommands.CommandRun {
			c := &putBatchRun{}
			c.SetDefaultFlags(defaultAuthOpts)
			return c
		},
	}
}

type putBatchRun struct {
	baseCommandRun
}

func (r *putBatchRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)
	if len(args) < 1 {
		return r.done(ctx, fmt.Errorf("missing parameter: <JSON Request>"))
	}

	var reqBody struct {
		Builds []json.RawMessage `json:"builds"`
	}
	for i, a := range args {
		aBytes := []byte(a)
		// verify that args are valid here before sending the request.
		if err := json.Unmarshal(aBytes, &buildbucket.ApiPutRequestMessage{}); err != nil {
			return r.done(ctx, fmt.Errorf("invalid build request #%d: %s", i, err))
		}
		reqBody.Builds = append(reqBody.Builds, json.RawMessage(aBytes))
	}

	return r.callAndDone(ctx, "PUT", "builds/batch", reqBody)
}
