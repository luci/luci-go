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
	"strconv"

	"github.com/google/uuid"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
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

func parsePutRequest(args []string, prefixFunc func() (string, error)) (*buildbucket.ApiPutBatchRequestMessage, error) {
	if len(args) <= 0 {
		return nil, fmt.Errorf("missing parameter: <JSON Request>")
	}

	req := &buildbucket.ApiPutBatchRequestMessage{}
	opIDPrefix := ""
	for i, a := range args {
		// verify that args are valid here before sending the request.
		putReq := &buildbucket.ApiPutRequestMessage{}
		if err := json.Unmarshal([]byte(a), putReq); err != nil {
			return nil, errors.Annotate(err, "invalid build request #%d", i).Err()
		}
		if putReq.ClientOperationId == "" {
			if opIDPrefix == "" {
				var err error
				opIDPrefix, err = prefixFunc()
				if err != nil {
					return nil, err
				}
			}
			putReq.ClientOperationId = opIDPrefix + "-" + strconv.Itoa(i)
		}
		req.Builds = append(req.Builds, putReq)
	}

	return req, nil
}

func (r *putBatchRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)

	req, err := parsePutRequest(args, func() (string, error) {
		id, err := uuid.NewRandom()
		return id.String(), err
	})
	if err != nil {
		return r.done(ctx, err)
	}

	client, err := r.createClient(ctx)
	if err != nil {
		return r.done(ctx, err)
	}

	resBytes, err := client.call(ctx, "PUT", "builds/batch", req)
	if err != nil {
		return r.done(ctx, err)
	}

	var res buildbucket.ApiPutBatchResponseMessage
	if err := json.Unmarshal(resBytes, &res); err != nil {
		return r.done(ctx, err)
	}

	hasErrors := false

	if res.Error != nil {
		hasErrors = true
		logging.Errorf(ctx, "Failed to schedule builds (reason: %s): %s",
			res.Error.Reason, res.Error.Message)
	}

	for i, r := range res.Results {
		if r.Error != nil {
			hasErrors = true
			logging.Errorf(ctx, "Failed to schedule build #%d (reason: %s): %s\n",
				i, r.Error.Reason, r.Error.Message)
		}
	}
	if hasErrors {
		return 1
	}

	fmt.Printf("%s\n", resBytes)
	return 0

}
