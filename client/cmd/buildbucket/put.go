// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"

	"github.com/maruel/subcommands"

	"github.com/luci/luci-go/common/api/buildbucket/buildbucket/v1"
	"github.com/luci/luci-go/common/cli"
	"github.com/luci/luci-go/common/logging"
)

var cmdPutBatch = &subcommands.Command{
	UsageLine: `put [flags] <JSON request>...`,
	ShortDesc: "schedule  builds",
	LongDesc: "Schedule builds. \n" +
		"See https://godoc.org/github.com/luci/luci-go/common/api/" +
		"buildbucket/buildbucket/v1#ApiPutBatchRequestMessage " +
		"for JSON request message schema.",
	CommandRun: func() subcommands.CommandRun {
		c := &putBatchRun{}
		c.SetDefaultFlags()
		return c
	},
}

type putBatchRun struct {
	baseCommandRun
}

func (r *putBatchRun) Run(a subcommands.Application, args []string) int {
	ctx := cli.GetContext(a, r)
	if len(args) < 1 {
		logging.Errorf(ctx, "missing parameter: <JSON Request>")
		return 1
	}

	reqMessage := &buildbucket.ApiPutBatchRequestMessage{}

	for i := range args {
		build := &buildbucket.ApiPutRequestMessage{}
		if err := json.Unmarshal([]byte(args[i]), build); err != nil {
			logging.Errorf(ctx, "could not unmarshal %s: %s", args[i], err)
			return 1
		}
		reqMessage.Builds = append(reqMessage.Builds, build)
	}

	service, err := r.makeService(ctx, a)
	if err != nil {
		return 1
	}

	response, err := service.PutBatch(reqMessage).Do()
	if err != nil {
		logging.Errorf(ctx, "buildbucket.PutBatch failed: %s", err)
		return 1
	}

	responseJSON, err := response.MarshalJSON()
	if err != nil {
		logging.Errorf(ctx, "could not marshal response: %s", err)
		return 1
	}
	fmt.Println(string(responseJSON))
	return 0
}
