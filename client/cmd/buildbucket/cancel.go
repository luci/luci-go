// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"fmt"
	"strconv"

	"github.com/maruel/subcommands"

	"github.com/luci/luci-go/common/cli"
	"github.com/luci/luci-go/common/logging"
)

var cmdCancel = &subcommands.Command{
	UsageLine: `cancel [flags] <build id>`,
	ShortDesc: "Cancel a build",
	LongDesc:  "Attempt to cancel an existing build.",
	CommandRun: func() subcommands.CommandRun {
		c := &cancelRun{}
		c.SetDefaultFlags()
		return c
	},
}

type cancelRun struct {
	baseCommandRun
}

func (r *cancelRun) Run(a subcommands.Application, args []string) int {
	ctx := cli.GetContext(a, r)
	if len(args) < 1 {
		logging.Errorf(ctx, "missing parameter: <Build ID>")
		return 1
	} else if len(args) > 1 {
		logging.Errorf(ctx, "unexpected arguments: %s", args[1:])
	}

	buildId, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		logging.Errorf(ctx, "expected a build id (int64): %s", err)
		return 1
	}

	service, err := r.makeService(ctx, a)
	if err != nil {
		return 1
	}

	response, err := service.Cancel(buildId).Do()
	if err != nil {
		logging.Errorf(ctx, "buildbucket.Cancel failed: %s", err)
		return 1
	}

	responseJSON, err := response.MarshalJSON()
	if err != nil {
		logging.Errorf(ctx, "could not unmarshal response: %s", err)
		return 1
	}

	fmt.Println(string(responseJSON))
	return 0
}
