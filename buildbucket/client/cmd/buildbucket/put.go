// Copyright 2016 The LUCI Authors.
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

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/auth"
	"go.chromium.org/luci/common/cli"
)

func cmdPutBatch(defaultAuthOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `put [flags] <JSON request>...`,
		ShortDesc: "schedule builds",
		LongDesc: "Schedule builds. \n" +
			"See https://godoc.org/go.chromium.org/luci/common/api/" +
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
