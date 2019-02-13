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
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/golang/protobuf/jsonpb"
	"github.com/maruel/subcommands"
	"google.golang.org/genproto/protobuf/field_mask"

	"go.chromium.org/luci/auth"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/cli"
)

func cmdCollect(defaultAuthOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `collect [flags] <build id> [<build id> [<build id> ...] ]`,
		ShortDesc: "wait for a list of builds to end and return their details",
		LongDesc: "wait for a list of builds to end and write their details " +
			"into an output file as JSONPB-encoded proto messages embedded into a " +
			"JSON Array.",
		CommandRun: func() subcommands.CommandRun {
			r := &collectRun{}
			r.SetDefaultFlags(defaultAuthOpts)
			r.Flags.DurationVar(
				&r.intervalArg, "interval", time.Minute,
				"time in seconds to wait between requests")
			r.Flags.StringVar(&r.outputArg, "output", "", "path to the file into "+
				"which build details are written; nothing is written if unspecified")
			return r
		},
	}
}

type collectRun struct {
	baseCommandRun
	repeatedBuildIDArg
	intervalArg time.Duration
	outputArg   string
}

func (r *collectRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)

	if err := r.parseArgs(args); err != nil {
		return r.done(ctx, err)
	}

	client, err := r.newClient(ctx)
	if err != nil {
		return r.done(ctx, err)
	}

	unfinishedBuilds := r.buildIDs
	endedBuildDetails := make([]*buildbucketpb.Build, 0, len(r.buildIDs))
	for {
		breq := &buildbucketpb.BatchRequest{}
		for _, buildID := range unfinishedBuilds {
			getReq := &buildbucketpb.GetBuildRequest{
				Id: buildID,
				Fields: &field_mask.FieldMask{
					Paths: []string{
						// Include all output properties, which are omitted by default.
						"output",

						// Since we add output props above, we must list default fields too.
						"builder",
						"cancel_reason",
						"create_time",
						"created_by",
						"end_time",
						"id",
						"input.experimental",
						"input.gerrit_changes",
						"input.gitiles_commit",
						"number",
						"start_time",
						"status",
						"update_time",
					},
				},
			}
			breq.Requests = append(breq.Requests, &buildbucketpb.BatchRequest_Request{
				Request: &buildbucketpb.BatchRequest_Request_GetBuild{
					GetBuild: getReq,
				},
			})
		}

		fmt.Printf("Checking build statuses... ")
		bresp, err := client.Batch(ctx, breq)
		if err != nil {
			return r.done(ctx, err)
		}

		unfinishedBuilds = nil
		for _, resp := range bresp.Responses {
			build := resp.GetGetBuild()
			if build.Status&buildbucketpb.Status_ENDED_MASK != 0 {
				endedBuildDetails = append(endedBuildDetails, build)
			} else {
				unfinishedBuilds = append(unfinishedBuilds, build.Id)
			}
		}

		if len(unfinishedBuilds) == 0 {
			fmt.Printf("all ended\n")
			break
		}

		fmt.Printf("%d still running/pending\n", len(unfinishedBuilds))
		fmt.Printf("Waiting for %s before trying again...\n", r.intervalArg)
		time.Sleep(r.intervalArg)
	}

	if r.outputArg != "" {
		fmt.Printf("Writing build details to %s\n", r.outputArg)
		outputFile, err := os.Create(r.outputArg)
		if err != nil {
			return r.done(ctx, err)
		}

		encodedMessages := make([]json.RawMessage, 0, len(endedBuildDetails))
		m := &jsonpb.Marshaler{}
		for _, build := range endedBuildDetails {
			messageString, err := m.MarshalToString(build)
			if err != nil {
				return r.done(ctx, err)
			}
			encodedMessages = append(encodedMessages, json.RawMessage(messageString))
		}

		endedBuildsEncoded, err := json.Marshal(encodedMessages)
		if err != nil {
			return r.done(ctx, err)
		}

		outputFile.Write(endedBuildsEncoded)
		outputFile.Close()
	}

	return r.done(ctx, nil)
}
